package gatesentryproxy

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"log"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

var blocked_image []byte

func CreateBlockedImageBytes() {
	log.Println("[IMAGE] Creating blocked image bytes")
	// read image from disk
	image_file, err := os.Open("blocked.jpg")
	if err != nil {
		log.Println("[IMAGE] Error opening image file:", err)
		return
	}

	// decode jpeg into image.Image
	image_file_jpeg, err := jpeg.Decode(image_file)
	if err != nil {
		log.Println("[IMAGE] Error decoding image file:", err)
		return
	}

	buf := new(bytes.Buffer)

	err = jpeg.Encode(buf, image_file_jpeg, nil)

	// write buf.Bytes() to disk

	blocked_image = buf.Bytes()
}

func createEmptyImage(width, height int, format string) ([]byte, error) {
	return blocked_image, nil
	// Create a new RGBA image with the specified width and height
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill the top half with red and the bottom half with blue
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if y < height/2 {
				img.Set(x, y, color.RGBA{255, 0, 0, 255}) // Red
			} else {
				img.Set(x, y, color.RGBA{0, 0, 255, 255}) // Blue
			}
		}
	}

	// Add the text "blocked" to the image
	addLabel(img, "blocked")

	// Use a bytes.Buffer to capture the encoded image data
	var buf bytes.Buffer

	// Encode the image to the desired format
	switch format {
	case "jpeg":
		err := jpeg.Encode(&buf, img, nil)
		return buf.Bytes(), err
	case "png":
		err := png.Encode(&buf, img)
		return buf.Bytes(), err
	case "gif":
		err := gif.Encode(&buf, img, &gif.Options{
			NumColors: 256,
			Quantizer: nil,
			Drawer:    nil,
		})
		return buf.Bytes(), err
	default:
		return nil, nil
	}
}

func addLabel(img *image.RGBA, label string) {
	col := color.RGBA{255, 255, 255, 255} // White color

	// Calculate the width and height of the text to be added
	labelWidth := font.MeasureString(basicfont.Face7x13, label).Round()
	labelHeight := basicfont.Face7x13.Metrics().Height.Round()

	// Calculate the starting point to draw the string such that it's centered
	pointX := (img.Bounds().Dx() - labelWidth) / 2
	pointY := (img.Bounds().Dy() + labelHeight) / 2

	point := fixed.Point26_6{fixed.Int26_6(pointX), fixed.Int26_6(pointY)}

	drawer := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: basicfont.Face7x13,
		Dot:  point,
	}
	drawer.DrawString(label)
}
