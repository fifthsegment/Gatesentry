package gatesentryproxy

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
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

func createImageWithText(texts []string) ([]byte, error) {
	// Create a new RGBA image with the desired size (512x512)
	img := image.NewRGBA(image.Rect(0, 0, 512, 512))

	// Set a background color (e.g., white)
	bgColor := color.White
	draw.Draw(img, img.Bounds(), &image.Uniform{C: bgColor}, image.Point{}, draw.Src)

	// Text color (e.g., black)
	textColor := color.Black

	// Calculate the width and height of the text to be added
	// labelWidth := font.MeasureString(basicfont.Face7x13, text).Round()
	// labelHeight := basicfont.Face7x13.Metrics().Height.Round()

	// Calculate the starting point to draw the text at the center
	// pointX := (img.Bounds().Dx() - labelWidth) / 2
	// pointY := (img.Bounds().Dy() - labelHeight) / 2

	for i, text := range texts {
		var nextLine = 1400 * i
		point := fixed.Point26_6{X: 100, Y: fixed.Int26_6(nextLine)}

		var fontFace = basicfont.Face7x13
		fontFace.Height = 80
		fontFace.Left = 20
		drawer := &font.Drawer{
			Dst:  img,
			Src:  image.NewUniform(textColor),
			Face: fontFace,
			Dot:  point,
		}
		drawer.DrawString(text)
	}

	// Encode the image as JPEG and return it as bytes
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func createTextOverlayOnImage(imageBytes []byte, texts []string) ([]byte, error) {
	// conver imageBytes to image
	img, _, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		log.Println("[IMAGE] Error decoding image file:", err)
		return nil, err
	}

	// Set a background color (e.g., white)
	// bgColor := color.White

	// Text color (e.g., black)
	textColor := color.Black

	for i, text := range texts {
		var nextLine = 1400 * i
		point := fixed.Point26_6{X: 100, Y: fixed.Int26_6(nextLine)}

		var fontFace = basicfont.Face7x13
		fontFace.Height = 80
		fontFace.Left = 20
		drawer := &font.Drawer{
			Dst:  img.(*image.RGBA),
			Src:  image.NewUniform(textColor),
			Face: fontFace,
			Dot:  point,
		}
		drawer.DrawString(text)
	}

	// Encode the image as JPEG and return it as bytes
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil

	// Encode the image as JPEG and return it as bytes
	// var buf bytes.Buffer
	// if err := jpeg.Encode(&buf, img, nil); err != nil {
	// 	return nil, err
	// }

	// return buf.Bytes(), nil

}

func createEmptyImage(width, height int, format string, texts []string) ([]byte, error) {
	var img1, _ = createImageWithText(texts)
	return img1, nil
}

func addLabel(img *image.RGBA, label string) {
	X := 100
	Y := 100
	col := color.RGBA{255, 255, 255, 255} // White color

	// Calculate the width of the text to be added
	// labelWidth := font.MeasureString(basicfont.Face7x13, label).Round()

	// Calculate the starting point to draw the string such that it's centered
	pointX := X
	pointY := Y

	point := fixed.Point26_6{fixed.Int26_6(pointX + X), fixed.Int26_6(pointY + Y)}

	drawer := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: basicfont.Face7x13,
		Dot:  point,
	}
	drawer.DrawString(label)
}
