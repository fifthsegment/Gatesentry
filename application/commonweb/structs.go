package gatesentry2comonweb

import(
	"os"
	"crypto/sha256"
	"encoding/hex"
	"io"
)

type GSDataUpdater struct{
	Email string
	Id string
}

type GSConsumptionUpdater struct{
	Id string
	Consumption int64
	Message string
	AdditionalInfo string
	Time string
}

type GSKeepAliver struct{
	Id string
	Version float32
}

type GSKeepAliveResponse struct{
	Ok bool
	Error bool
	Message string
}

type GSConsumptionUpdaterResponse struct{
	Ok bool
	Error bool
	Message string
	AdditionalInfo string
}

func GetFileHash( binpath string )string{

	f, err := os.Open(binpath)
	if err != nil {
		return "";
	}
	defer f.Close();

	h:= sha256.New();
	if _, err := io.Copy(h,f); err != nil {
		return "";
	}
	// encoded := base64.StdEncoding.EncodeToString([]byte( h.Sum(nil) ))
	encoded := hex.EncodeToString(h.Sum(nil))

	return encoded;
}