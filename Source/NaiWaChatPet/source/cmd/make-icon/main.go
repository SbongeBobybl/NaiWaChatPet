package main

import (
	"bytes"
	"codexpet/reconstruction/compositor"
	"encoding/binary"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) != 3 {
		panic("make-icon source-body output-folder")
	}
	f, e := os.Open(os.Args[1])
	if e != nil {
		panic(e)
	}
	im, e := png.Decode(f)
	f.Close()
	if e != nil {
		panic(e)
	}
	b := im.Bounds()
	box := image.Rectangle{Min: b.Max}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := im.At(x, y).RGBA()
			if a > 0 {
				box = box.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	// Original pet pixels, including both eyes. Make a square, not a stretched body.
	crop := image.NewNRGBA(image.Rect(0, 0, box.Dx(), box.Dy()))
	draw.Draw(crop, crop.Bounds(), im, box.Min, draw.Src)
	outDir := os.Args[2]
	os.MkdirAll(outDir, 0700)
	sizes := []int{16, 24, 32, 48, 64, 128, 256}
	images := make([][]byte, 0, len(sizes))
	for _, size := range sizes {
		w := max(1, crop.Bounds().Dx()*(size-2)/crop.Bounds().Dy())
		h := size - 2
		body, e := compositor.Resize(crop, w, h)
		if e != nil {
			panic(e)
		}
		canvas := image.NewNRGBA(image.Rect(0, 0, size, size))
		draw.Draw(canvas, image.Rect((size-w)/2, 1, (size-w)/2+w, 1+h), body, image.Point{}, draw.Src)
		var buffer bytes.Buffer
		png.Encode(&buffer, canvas)
		images = append(images, buffer.Bytes())
		os.WriteFile(filepath.Join(outDir, "icon-"+stringNumber(size)+".png"), buffer.Bytes(), 0600)
	}
	var ico bytes.Buffer
	binary.Write(&ico, binary.LittleEndian, uint16(0))
	binary.Write(&ico, binary.LittleEndian, uint16(1))
	binary.Write(&ico, binary.LittleEndian, uint16(len(sizes)))
	offset := 6 + 16*len(sizes)
	for i, size := range sizes {
		ico.Write([]byte{byte(size % 256), byte(size % 256), 0, 0})
		binary.Write(&ico, binary.LittleEndian, uint16(1))
		binary.Write(&ico, binary.LittleEndian, uint16(32))
		binary.Write(&ico, binary.LittleEndian, uint32(len(images[i])))
		binary.Write(&ico, binary.LittleEndian, uint32(offset))
		offset += len(images[i])
	}
	for _, data := range images {
		ico.Write(data)
	}
	if e := os.WriteFile(filepath.Join(outDir, "NaiWaChatPet.ico"), ico.Bytes(), 0600); e != nil {
		panic(e)
	}
}
func stringNumber(n int) string {
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
