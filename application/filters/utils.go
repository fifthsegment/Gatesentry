package gatesentry2filters;

import (
	"github.com/antonholmquist/jason"
	"strconv"
	"io/ioutil"
	"log"
	"time"
	"math/rand"
	"os"
	"strings"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const (
    letterIdxBits = 6                    // 6 bits to represent a letter index
    letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
    letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

func exists(path string) (bool, error) {
    _, err := os.Stat(path)
    if err == nil { return true, nil }
    if os.IsNotExist(err) { return false, nil }
    return true, err
}

func MakeFile(name string){
	// if _, err := os.Stat(f); os.IsNotExist(err) {

		// log.Println("File not found = " + GSBASEDIR )
	newname := strings.Replace(name, GSBASEDIR, "", -1 )
		data, err := Asset(newname)
		if err != nil {
			// handle error here
			log.Println( "Error = "+err.Error() )
		}else{
			log.Println("Creating a new file")
			_=ioutil.WriteFile(name, data, 0777)
			// log.Println( err.Error() )
		}
	// }
}

func(f *GSFilter) LoadFilterFile( ){
	f.FileContents = []GSFILTERLINE{};
	filepath :=  f.FileName;
	log.Println( "Loading file = " + filepath )
	b , err  := ioutil.ReadFile(filepath)
	if err != nil {
		MakeFile(filepath)
		// fmt.Println("Unable to read file : " + filepath)
    	// return;
    	b , err  = ioutil.ReadFile(filepath)
    }
    v, err := jason.NewObjectFromBytes(b)
    if err != nil {
    	return;
    }
    log.Println( "Reading file data" )
    keywords, err := v.GetObjectArray("keywords")
	for _, keyword := range keywords {
	  stopword, err := keyword.GetString("Content")
	  if ( err != nil ){
	  	continue;
	  }
	  pointsNumber, err := keyword.GetNumber("Score")
	  points := pointsNumber.String();
	  if ( err!=nil ){
	  	points = "0";
	  }
	  num , err :=strconv.Atoi( points )
	  x := GSFILTERLINE{Content: stopword, Score: num }
	  _=x;
	  
	  // R.Logger.Debug( "Adding = " + stopword );
	  f.FileContents = append(f.FileContents, x)
	}
}

func GSSaveFilterFile( file string, content string ){
	str:= `{"keywords":`+ content +`}`;
	GSFileSaver( file, str );
}


func GSFileSaver( file string, content string ){
	log.Println("Saving file = " + file );
	err:=ioutil.WriteFile( file , []byte(content), 0644 )
	// GSMirrorGSFiletoSquid(filename, squidfilename )
	if ( err != nil ){
		// R.Logger.Error("Unable to save file ")
	}else{
		// LoadAllFiles(R)
	}
}



func RandStringGenerator(n int) string {
	var src = rand.NewSource(time.Now().UnixNano())
    b := make([]byte, n)
    // A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
    for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
        if remain == 0 {
            cache, remain = src.Int63(), letterIdxMax
        }
        if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
            b[i] = letterBytes[idx]
            i--
        }
        cache >>= letterIdxBits
        remain--
    }

    return string(b)
}