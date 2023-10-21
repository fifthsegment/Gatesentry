package gatesentry2filters

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"

	"bitbucket.org/abdullah_irfan/gatesentryproxy"

	"golang.org/x/image/webp"
)

type InferenceDetectionCategory struct {
	Class string  `json:"class"`
	Score float64 `json:"score"`
}

type InferenceResponse struct {
	Category   string                       `json:"category"`
	Confidence int                          `json:"confidence"`
	Detections []InferenceDetectionCategory `json:"detections"`
}

var contentTypeToExt = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/jpg":  ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
	"image/avif": ".avif",
	"":           "",
}

func ConvertWebPToJPEG(webpData []byte) ([]byte, error) {
	// Decode webp bytes to image.Image
	img, err := webp.Decode(bytes.NewReader(webpData))
	if err != nil {
		return nil, err
	}

	// Encode image.Image to jpeg
	var jpegBuf bytes.Buffer
	err = jpeg.Encode(&jpegBuf, img, nil)
	if err != nil {
		return nil, err
	}

	return jpegBuf.Bytes(), nil
}

func FilterImagesAI(gafd *gatesentryproxy.GSContentFilterData, ai_service_url string) {
	// if R.GSSettings.Get("enable_ai_image_filtering") == "true" && R.GSSettings.Get("ai_scanner_url") != "" {
	// convert bytes to json struct of type ContentScannerInput
	// var contentScannerInput gatesentryproxy.ContentScannerInput
	// err := json.Unmarshal(*bytesReceived, &contentScannerInput)
	// if err != nil {
	// 	log.Println("Error unmarshalling content scanner input")
	// }
	// log.Println("Running content scanner for content type = " + contentScannerInput.ContentType)

	if len(gafd.Content) < 6000 {
		// continue
	} else if (gafd.ContentType == "image/jpeg") || (gafd.ContentType == "image/jpg") || (gafd.ContentType == "image/png") || (gafd.ContentType == "image/gif") || (gafd.ContentType == "image/webp") || (gafd.ContentType == "image/avif") {
		contentType := gafd.ContentType
		log.Println("Running content scanner for image")

		// if contentType == "image/jpg" || contentType == "image/jpeg" || contentType == "image/png" || contentType == "image/gif" || contentType == "image/webp" {
		var b bytes.Buffer
		wr := multipart.NewWriter(&b)
		// part, _ := wr.CreateFormFile("image", "uploaded_image"+contentTypeToExt[contentType])

		if contentType == "image/webp" {
			// convert webp to jpeg
			jpegBytes, err := ConvertWebPToJPEG(gafd.Content)
			if err != nil {
				fmt.Println("Error converting webp to jpeg")
			}
			gafd.Content = jpegBytes
			contentType = "image/jpeg"
		}

		// Create a new form header for the file

		h := make(textproto.MIMEHeader)
		// ext := contentTypeToExt[contentType]
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image"; filename="%s"`, "uploaded_image"))
		part, _ := wr.CreatePart(h)

		part.Write(*&gafd.Content)

		b.Bytes()
		wr.Close()

		resp, _ := http.Post(ai_service_url, wr.FormDataContentType(), &b)
		if resp.StatusCode == http.StatusOK {
			// bytesLength := len(*gafd.Content)
			// convert bytes length to string
			//
			// bytesLengthString := strconv.Itoa(bytesLength)
			log.Println("Inference for " + gafd.Url + " Content type = " + contentType + "Length = " + strconv.Itoa(len(gafd.Content)) + " succeeded")
			respBytes, _ := io.ReadAll(resp.Body)
			responseString := string(respBytes)
			var inferenceResponse InferenceResponse
			err := json.Unmarshal([]byte(respBytes), &inferenceResponse)
			if err != nil {
				fmt.Println("Error:", err)
				return
			}
			log.Println("Inference Response = " + responseString)
			if inferenceResponse.Category == "sexy" && inferenceResponse.Confidence > 85 {
			}
			if inferenceResponse.Category == "porn" && inferenceResponse.Confidence > 85 {
			}
			if len(inferenceResponse.Detections) > 0 {
				var reasonForBlock []string
				var conditionsMet = 0

				for _, detection := range inferenceResponse.Detections {

					if detection.Class == "FEMALE_GENITALIA_EXPOSED" && detection.Score > 0.4 {
						reasonForBlock = append(reasonForBlock, " - "+detection.Class+" ("+strconv.FormatFloat(detection.Score, 'f', 2, 64)+")")
						conditionsMet += 2
					}
					if detection.Class == "FEMALE_BREAST_EXPOSED" && detection.Score > 0.4 {
						reasonForBlock = append(reasonForBlock, " - "+detection.Class+" ("+strconv.FormatFloat(detection.Score, 'f', 2, 64)+")")
						conditionsMet += 2
					}
					if detection.Class == "FEMALE_BREAST_COVERED" && detection.Score > 0.4 {
						reasonForBlock = append(reasonForBlock, " - "+detection.Class+" ("+strconv.FormatFloat(detection.Score, 'f', 2, 64)+")")
						conditionsMet += 1
					}
					if detection.Class == "BELLY_COVERED" && detection.Score > 0.5 {
						reasonForBlock = append(reasonForBlock, " - "+detection.Class+" ("+strconv.FormatFloat(detection.Score, 'f', 2, 64)+")")
						conditionsMet += 2
					}
					if detection.Class == "ARMPITS_EXPOSED" && detection.Score > 0.5 {
						reasonForBlock = append(reasonForBlock, " - "+detection.Class+" ("+strconv.FormatFloat(detection.Score, 'f', 2, 64)+")")
						conditionsMet++
					}
					if detection.Class == "MALE_GENITALIA_EXPOSED" && detection.Score > 0.5 {
						reasonForBlock = append(reasonForBlock, " - "+detection.Class+" ("+strconv.FormatFloat(detection.Score, 'f', 2, 64)+")")
						conditionsMet += 2
					}
					if detection.Class == "MALE_BREAST_EXPOSED" && detection.Score > 0.5 {
						reasonForBlock = append(reasonForBlock, " - "+detection.Class+" ("+strconv.FormatFloat(detection.Score, 'f', 2, 64)+")")
						conditionsMet++
					}

					if detection.Class == "BUTTOCKS_EXPOSED" && detection.Score > 0.5 {
						reasonForBlock = append(reasonForBlock, " - "+detection.Class+" ("+strconv.FormatFloat(detection.Score, 'f', 2, 64)+")")
						conditionsMet += 2
					}

					if detection.Class == "ANUS_EXPOSED" && detection.Score > 0.5 {
						reasonForBlock = append(reasonForBlock, " - "+detection.Class+" ("+strconv.FormatFloat(detection.Score, 'f', 2, 64)+")")
						conditionsMet += 2
					}

					if detection.Class == "BELLY_EXPOSED" && detection.Score > 0.5 {
						reasonForBlock = append(reasonForBlock, " - "+detection.Class+" ("+strconv.FormatFloat(detection.Score, 'f', 2, 64)+")")
						conditionsMet++
					}

				}
				// if conditionsMet >= 2 {
				// 	changed = true
				// }
				if conditionsMet >= 2 {
					jsonData, _ := json.Marshal(reasonForBlock)
					gafd.FilterResponseAction = gatesentryproxy.ProxyActionBlockedMediaContent
					gafd.FilterResponse = jsonData
				}

			}

		} else {
			fmt.Println("Inference for Content type = " + contentType + " failed")
			respBytes, _ := io.ReadAll(resp.Body)

			fmt.Println("Inference Response = " + string(respBytes))
		}
		defer resp.Body.Close()

	}
	// }
	// }
}

// func loadfilter(){
// 	gatesentry2.NewGSFilter("text/html", "filterfiles/stopwords.json")
// }
