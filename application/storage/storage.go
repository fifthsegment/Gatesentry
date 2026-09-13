package gatesentry2storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

const ENCRYPTIONKEY = "AES256Key-A23AS98BVM94PO3XSD10AA"

var GSBASEDIR = "./gatesentry"

// Store owns an in-memory byte snapshot and its durable file. All access to
// the snapshot is synchronized, and returned bytes are copies.
type Store struct {
	Id            string
	Encrypted     bool
	Encryptionkey string
	mu            sync.RWMutex
	data          []byte
	path          string
	loadErr       error
	pathMu        *sync.RWMutex
}

var pathLocks = struct {
	sync.Mutex
	locks map[string]*sync.RWMutex
}{locks: make(map[string]*sync.RWMutex)}

func lockForPath(path string) *sync.RWMutex {
	pathLocks.Lock()
	defer pathLocks.Unlock()
	cleanPath, err := filepath.Abs(path)
	if err != nil {
		cleanPath = filepath.Clean(path)
	}
	lock := pathLocks.locks[cleanPath]
	if lock == nil {
		lock = &sync.RWMutex{}
		pathLocks.locks[cleanPath] = lock
	}
	return lock
}

func SetBaseDir(dir string) { GSBASEDIR = dir }

// OpenStore loads a store and reports malformed, decryption, and filesystem
// errors. A missing file is a valid empty store and is not created until Set.
func OpenStore(name string, encrypt bool) (*Store, error) {
	path := GSBASEDIR + name
	s := &Store{Id: name, Encrypted: encrypt, Encryptionkey: ENCRYPTIONKEY, path: path, pathMu: lockForPath(path)}
	if err := s.Load(); err != nil {
		s.loadErr = err
		return s, err
	}
	return s, nil
}

// NewStore is retained for source compatibility. Err exposes a failed load.
func NewStore(name string, encrypt bool) *Store {
	s, _ := OpenStore(name, encrypt)
	return s
}

func (s *Store) Err() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadErr
}

func decodeEnvelope(data []byte, encryptionKey string) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("storage file is empty")
	}
	var envelope map[string]string
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("parse storage envelope: %w", err)
	}
	payload, ok := envelope["data"]
	if !ok {
		return nil, errors.New("parse storage envelope: missing data field")
	}
	encrypted, ok := envelope["encrypted"]
	if !ok {
		return nil, errors.New("parse storage envelope: missing encrypted field")
	}
	switch encrypted {
	case "false":
		return []byte(payload), nil
	case "true":
		// Continue below and decrypt the payload.
	default:
		return nil, fmt.Errorf("parse storage envelope: invalid encrypted value %q", encrypted)
	}
	plaintext, err := Decrypt([]byte(payload), []byte(encryptionKey))
	if err != nil {
		return nil, fmt.Errorf("decrypt storage payload: %w", err)
	}
	return plaintext, nil
}

func (s *Store) Load() error {
	s.pathMu.Lock()
	defer s.pathMu.Unlock()
	return s.loadLocked()
}

func (s *Store) loadLocked() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		s.data = nil
		s.loadErr = nil
		return nil
	}
	if err != nil {
		s.loadErr = fmt.Errorf("read storage %q: %w", s.Id, err)
		return s.loadErr
	}
	payload, err := decodeEnvelope(data, s.Encryptionkey)
	if err != nil {
		s.loadErr = fmt.Errorf("load storage %q: %w", s.Id, err)
		return s.loadErr
	}
	s.data = append([]byte(nil), payload...)
	s.loadErr = nil
	return nil
}

func encodeEnvelope(data []byte, encrypted bool, encryptionKey string) ([]byte, error) {
	payload := data
	encryptedValue := "false"
	if encrypted {
		var err error
		payload, err = Encrypt(data, []byte(encryptionKey))
		if err != nil {
			return nil, fmt.Errorf("encrypt storage payload: %w", err)
		}
		encryptedValue = "true"
	}
	b, err := json.Marshal(map[string]string{"data": string(payload), "encrypted": encryptedValue})
	if err != nil {
		return nil, fmt.Errorf("serialize storage envelope: %w", err)
	}
	return b, nil
}

var renameFile = os.Rename
var writeTemporaryFile = func(file *os.File, data []byte) error {
	_, err := io.Copy(file, bytes.NewReader(append([]byte(nil), data...)))
	return err
}
var syncFile = func(file *os.File) error { return file.Sync() }
var closeFile = func(file *os.File) error { return file.Close() }
var openDirectory = func(path string) (*os.File, error) { return os.Open(path) }

// atomicWrite prepares the complete replacement beside the destination. Files
// are always mode 0600. Failures before rename leave the destination intact.
// Once rename succeeds, committed is true even when syncing or closing the
// directory fails: the new destination is visible, though crash durability is
// uncertain and the caller still receives the material filesystem error.
func atomicWrite(path string, data []byte) (committed bool, err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return false, fmt.Errorf("create temporary storage file: %w", err)
	}
	tmpName := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			if closeErr := closeFile(tmp); err == nil && closeErr != nil {
				err = fmt.Errorf("close temporary storage file: %w", closeErr)
			}
		}
		if removeErr := os.Remove(tmpName); err == nil && removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = fmt.Errorf("clean temporary storage file: %w", removeErr)
		}
	}()
	if err = tmp.Chmod(0600); err != nil {
		return false, fmt.Errorf("set temporary storage permissions: %w", err)
	}
	if err = writeTemporaryFile(tmp, data); err != nil {
		return false, fmt.Errorf("write temporary storage file: %w", err)
	}
	if err = syncFile(tmp); err != nil {
		return false, fmt.Errorf("sync temporary storage file: %w", err)
	}
	if err = closeFile(tmp); err != nil {
		return false, fmt.Errorf("close temporary storage file: %w", err)
	}
	closed = true
	if err = renameFile(tmpName, path); err != nil {
		return false, fmt.Errorf("replace storage file: %w", err)
	}
	committed = true
	directory, err := openDirectory(dir)
	if err != nil {
		return true, fmt.Errorf("open storage directory for sync: %w", err)
	}
	if err = syncFile(directory); err != nil {
		_ = closeFile(directory)
		return true, fmt.Errorf("sync storage directory: %w", err)
	}
	if err = closeFile(directory); err != nil {
		return true, fmt.Errorf("close storage directory: %w", err)
	}
	return true, nil
}

func (s *Store) persistLocked(data []byte) (err error) {
	if s.loadErr != nil {
		return s.loadErr
	}
	envelope, err := encodeEnvelope(data, s.Encrypted, s.Encryptionkey)
	if err != nil {
		return fmt.Errorf("persist storage %q: %w", s.Id, err)
	}
	committed, err := atomicWrite(s.path, envelope)
	if committed {
		// Rename is the commit point. Keep memory consistent with the visible
		// destination even when the following directory durability step fails.
		s.data = append([]byte(nil), data...)
	}
	if err != nil {
		return fmt.Errorf("persist storage %q: %w", s.Id, err)
	}
	return nil
}

func (s *Store) Persist() error {
	s.pathMu.Lock()
	defer s.pathMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.persistLocked(s.data)
}

func (s *Store) Set(data []byte) error {
	s.pathMu.Lock()
	defer s.pathMu.Unlock()
	return s.setLocked(data)
}

func (s *Store) setLocked(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.persistLocked(append([]byte(nil), data...))
}

func (s *Store) Get() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]byte(nil), s.data...)
}

// MapStore serializes complete map read-modify-write transactions with every
// other Store opened for the same destination path. Each operation refreshes
// its snapshot under that shared path lock before reading or writing.
type MapStore struct {
	baseStore *Store
}

func OpenMapStore(name string, encrypt bool) (*MapStore, error) {
	s, err := OpenStore(name, encrypt)
	m := &MapStore{baseStore: s}
	if err != nil {
		return m, err
	}
	if _, err := parseMap(s.Get()); err != nil {
		err = fmt.Errorf("load map storage %q: %w", name, err)
		s.mu.Lock()
		s.loadErr = err
		s.mu.Unlock()
		return m, err
	}
	return m, nil
}

func NewMapStore(name string, encrypt bool) *MapStore {
	m, _ := OpenMapStore(name, encrypt)
	return m
}

func (m *MapStore) Err() error { return m.baseStore.Err() }

func parseMap(data []byte) (map[string]string, error) {
	values := make(map[string]string)
	if len(data) == 0 {
		return values, nil
	}
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("parse map storage: %w", err)
	}
	if values == nil {
		return nil, errors.New("parse map storage: payload must be a JSON object")
	}
	return values, nil
}

// UpdateValue atomically reads, transforms, and persists one value while the
// map lock is held. The previous value remains in memory and on disk on error.
func (m *MapStore) UpdateValue(key string, update func(string) (string, error)) error {
	m.baseStore.pathMu.Lock()
	defer m.baseStore.pathMu.Unlock()
	if err := m.baseStore.loadLocked(); err != nil {
		return err
	}
	if err := m.baseStore.Err(); err != nil {
		return err
	}
	values, err := parseMap(m.baseStore.Get())
	if err != nil {
		return err
	}
	next, err := update(values[key])
	if err != nil {
		return err
	}
	values[key] = next
	data, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("serialize map storage: %w", err)
	}
	return m.baseStore.setLocked(data)
}

func (m *MapStore) Update(key, value string) error {
	return m.UpdateValue(key, func(string) (string, error) { return value, nil })
}

// UpdateValues persists all supplied keys as one map transaction. Either the
// complete set reaches memory and disk, or the previous snapshot is retained.
func (m *MapStore) UpdateValues(updates map[string]string) error {
	m.baseStore.pathMu.Lock()
	defer m.baseStore.pathMu.Unlock()
	if err := m.baseStore.loadLocked(); err != nil {
		return err
	}
	if err := m.baseStore.Err(); err != nil {
		return err
	}
	values, err := parseMap(m.baseStore.Get())
	if err != nil {
		return err
	}
	for key, value := range updates {
		values[key] = value
	}
	data, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("serialize map storage: %w", err)
	}
	return m.baseStore.setLocked(data)
}

func (m *MapStore) GetE(key string) (string, error) {
	m.baseStore.pathMu.Lock()
	defer m.baseStore.pathMu.Unlock()
	if err := m.baseStore.loadLocked(); err != nil {
		// loadLocked leaves the synchronized in-memory snapshot unchanged on
		// failure. Return that known-good value with the observable error so
		// long-running policy callers can retain their last valid decision.
		values, snapshotErr := parseMap(m.baseStore.Get())
		if snapshotErr != nil {
			return "", errors.Join(err, snapshotErr)
		}
		return values[key], err
	}
	if err := m.baseStore.Err(); err != nil {
		return "", err
	}
	values, err := parseMap(m.baseStore.Get())
	if err != nil {
		return "", err
	}
	return values[key], nil
}

// GetOrDefault is the explicit compatibility path for callers where a
// fallback is safe. Persistence errors are intentionally mapped to fallback.
// User-facing and security-sensitive paths must use GetE.
func (m *MapStore) GetOrDefault(key, fallback string) string {
	value, err := m.GetE(key)
	if err != nil {
		return fallback
	}
	return value
}

func (m *MapStore) GetIntE(key string) (int, error) {
	value, err := m.GetE(key)
	if err != nil {
		return 0, err
	}
	i, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse integer map value %q: %w", key, err)
	}
	return i, nil
}

func (m *MapStore) SetDefault(key, value string) error {
	m.baseStore.pathMu.Lock()
	defer m.baseStore.pathMu.Unlock()
	if err := m.baseStore.loadLocked(); err != nil {
		return err
	}
	if err := m.baseStore.Err(); err != nil {
		return err
	}
	values, err := parseMap(m.baseStore.Get())
	if err != nil {
		return err
	}
	if _, exists := values[key]; exists {
		return nil
	}
	values[key] = value
	data, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("serialize map storage: %w", err)
	}
	return m.baseStore.setLocked(data)
}
