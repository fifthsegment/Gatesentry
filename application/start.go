package gatesentryf

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	gatesentryDnsServer "bitbucket.org/abdullah_irfan/gatesentryf/dns/server"
	gatesentryWebserver "bitbucket.org/abdullah_irfan/gatesentryf/webserver"
)

var R *GSRuntime

var appLifecycle struct {
	sync.Mutex
	boundAddress *string
	webServer    *http.Server
	webListener  net.Listener
	dnsDone      chan struct{}
	stopOnce     sync.Once
	stopDone     chan struct{}
	stopErr      error
	started      bool
}

func SetDNSControllerDone(done chan struct{}) {
	appLifecycle.Lock()
	appLifecycle.dnsDone = done
	appLifecycle.Unlock()
}

func SetBoundAddress(address *string) {
	appLifecycle.Lock()
	defer appLifecycle.Unlock()
	appLifecycle.boundAddress = address
}

func Start(webadminport int) (*GSRuntime, error) {
	GSVerString := GetApplicationVersion()
	fmt.Println("Starting GateSentry v " + GSVerString)
	appLifecycle.Lock()
	boundAddress := appLifecycle.boundAddress
	appLifecycle.Unlock()
	if boundAddress == nil {
		emptyAddress := ""
		boundAddress = &emptyAddress
	}
	R = &GSRuntime{
		WebServerPort:    webadminport,
		FilterFiles:      make(map[string]string),
		DNSServerChannel: make(chan int, 1),
		BoundAddress:     boundAddress,
	}
	if err := R.Init(); err != nil {
		closeLoggerAfterStartFailure(R)
		return nil, err
	}
	// Validate and apply first-run authentication before any network service
	// starts. Unsafe or malformed unattended configuration must fail startup.
	if _, err := gatesentryWebserver.NewAuthManager(R.GSSettings, os.Getenv("GATESENTRY_BOOTSTRAP_FILE")); err != nil {
		closeLoggerAfterStartFailure(R)
		return nil, fmt.Errorf("initialize administrator authentication: %w", err)
	}
	LoadFilters()

	handler, err := webServerHandler(webadminport)
	if err != nil {
		closeLoggerAfterStartFailure(R)
		return nil, err
	}
	webServer := &http.Server{Addr: ":" + strconv.Itoa(webadminport), Handler: handler}
	webListener, err := net.Listen("tcp", webServer.Addr)
	if err != nil {
		closeLoggerAfterStartFailure(R)
		return nil, fmt.Errorf("listen for web administration: %w", err)
	}
	appLifecycle.Lock()
	appLifecycle.webServer = webServer
	appLifecycle.webListener = webListener
	appLifecycle.dnsDone = nil
	appLifecycle.stopOnce = sync.Once{}
	appLifecycle.stopDone = make(chan struct{})
	appLifecycle.stopErr = nil
	appLifecycle.started = true
	appLifecycle.Unlock()

	fmt.Println("Starting GateSentry webserver on port " + strconv.Itoa(R.WebServerPort))
	go func() {
		if err := webServer.Serve(webListener); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			fmt.Printf("Webserver stopped: %v\n", err)
		}
	}()

	return R, nil
}

func closeLoggerAfterStartFailure(runtime *GSRuntime) {
	if runtime == nil || runtime.Logger == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = runtime.Logger.Close(ctx)
}

// Stop stops producers before draining and closing the activity logger.
func Stop(ctx context.Context) error {
	fmt.Println("Stopping GateSentry " + GSVerString)
	appLifecycle.Lock()
	webServer := appLifecycle.webServer
	dnsDone := appLifecycle.dnsDone
	stopDone := appLifecycle.stopDone
	if !appLifecycle.started || stopDone == nil {
		appLifecycle.Unlock()
		return nil
	}
	appLifecycle.stopOnce.Do(func() {
		go func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			var stopErr error
			if webServer != nil {
				if err := webServer.Shutdown(shutdownCtx); err != nil {
					stopErr = errors.Join(stopErr, fmt.Errorf("stop web server: %w", err))
					_ = webServer.Close()
				}
			}
			gatesentryDnsServer.StopDNSServer()
			if dnsDone != nil {
				select {
				case <-dnsDone:
				case <-shutdownCtx.Done():
					stopErr = errors.Join(stopErr, fmt.Errorf("wait for DNS controller: %w", shutdownCtx.Err()))
				}
			}
			gatesentryDnsServer.StopPolicyEnforcement()
			StopBonjour()
			if R != nil && R.Logger != nil {
				if err := R.Logger.Flush(shutdownCtx); err != nil {
					stopErr = errors.Join(stopErr, fmt.Errorf("flush activity logger: %w", err))
				}
				if err := R.Logger.Close(shutdownCtx); err != nil {
					stopErr = errors.Join(stopErr, fmt.Errorf("close activity logger: %w", err))
				}
			}
			appLifecycle.Lock()
			appLifecycle.stopErr = stopErr
			appLifecycle.started = false
			close(stopDone)
			appLifecycle.Unlock()
		}()
	})
	appLifecycle.Unlock()

	select {
	case <-stopDone:
		appLifecycle.Lock()
		err := appLifecycle.stopErr
		appLifecycle.Unlock()
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
