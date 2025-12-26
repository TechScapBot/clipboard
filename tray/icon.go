package tray

import (
	"bytes"
	"encoding/binary"
)

// getIcon returns a valid ICO icon for the system tray (Windows needs ICO format)
func getIcon() []byte {
	return createClipboardICO()
}

// createClipboardICO creates a 16x16 ICO icon programmatically
func createClipboardICO() []byte {
	const size = 16

	// Create BGRA pixel data (ICO uses BGRA, bottom-to-top)
	pixels := make([][4]byte, size*size)

	// Colors (BGRA format)
	green := [4]byte{0, 150, 0, 255}       // Clipboard border
	white := [4]byte{255, 255, 255, 255}   // Inner area
	darkGreen := [4]byte{0, 100, 0, 255}   // Lines/clip
	transparent := [4]byte{0, 0, 0, 0}     // Transparent

	// Initialize all to transparent
	for i := range pixels {
		pixels[i] = transparent
	}

	// Helper to set pixel (y is from top in our logic, will flip later)
	setPixel := func(x, y int, c [4]byte) {
		if x >= 0 && x < size && y >= 0 && y < size {
			pixels[y*size+x] = c
		}
	}

	// Draw clipboard body (green border)
	for y := 3; y < 16; y++ {
		for x := 1; x < 15; x++ {
			setPixel(x, y, green)
		}
	}

	// Draw white inner area
	for y := 5; y < 15; y++ {
		for x := 2; x < 14; x++ {
			setPixel(x, y, white)
		}
	}

	// Draw clip at top (darker green)
	for x := 5; x < 11; x++ {
		setPixel(x, 2, darkGreen)
		setPixel(x, 3, darkGreen)
	}
	for x := 6; x < 10; x++ {
		setPixel(x, 1, darkGreen)
	}

	// Draw lines on clipboard (text effect)
	for x := 4; x < 12; x++ {
		setPixel(x, 7, darkGreen)
		setPixel(x, 9, darkGreen)
		setPixel(x, 11, darkGreen)
	}

	return encodeICO(size, pixels)
}

// encodeICO creates a valid ICO file from pixel data
func encodeICO(size int, pixels [][4]byte) []byte {
	var buf bytes.Buffer

	// ICO Header (6 bytes)
	binary.Write(&buf, binary.LittleEndian, uint16(0))     // Reserved
	binary.Write(&buf, binary.LittleEndian, uint16(1))     // Type: 1 = ICO
	binary.Write(&buf, binary.LittleEndian, uint16(1))     // Number of images

	// Calculate sizes
	bmpInfoHeaderSize := 40
	pixelDataSize := size * size * 4         // BGRA
	andMaskRowSize := ((size + 31) / 32) * 4 // AND mask row size (4-byte aligned)
	andMaskSize := andMaskRowSize * size
	imageDataSize := bmpInfoHeaderSize + pixelDataSize + andMaskSize
	imageDataOffset := 6 + 16 // ICO header + 1 directory entry

	// ICO Directory Entry (16 bytes)
	buf.WriteByte(byte(size))           // Width (0 means 256)
	buf.WriteByte(byte(size))           // Height
	buf.WriteByte(0)                    // Color palette (0 = no palette)
	buf.WriteByte(0)                    // Reserved
	binary.Write(&buf, binary.LittleEndian, uint16(1))  // Color planes
	binary.Write(&buf, binary.LittleEndian, uint16(32)) // Bits per pixel
	binary.Write(&buf, binary.LittleEndian, uint32(imageDataSize))
	binary.Write(&buf, binary.LittleEndian, uint32(imageDataOffset))

	// BITMAPINFOHEADER (40 bytes)
	binary.Write(&buf, binary.LittleEndian, uint32(40))        // Header size
	binary.Write(&buf, binary.LittleEndian, int32(size))       // Width
	binary.Write(&buf, binary.LittleEndian, int32(size*2))     // Height (doubled for XOR+AND masks)
	binary.Write(&buf, binary.LittleEndian, uint16(1))         // Planes
	binary.Write(&buf, binary.LittleEndian, uint16(32))        // Bits per pixel
	binary.Write(&buf, binary.LittleEndian, uint32(0))         // Compression (none)
	binary.Write(&buf, binary.LittleEndian, uint32(pixelDataSize+andMaskSize)) // Image size
	binary.Write(&buf, binary.LittleEndian, int32(0))          // X pixels per meter
	binary.Write(&buf, binary.LittleEndian, int32(0))          // Y pixels per meter
	binary.Write(&buf, binary.LittleEndian, uint32(0))         // Colors used
	binary.Write(&buf, binary.LittleEndian, uint32(0))         // Important colors

	// Pixel data (BGRA, bottom-to-top)
	for y := size - 1; y >= 0; y-- {
		for x := 0; x < size; x++ {
			p := pixels[y*size+x]
			buf.WriteByte(p[0]) // B
			buf.WriteByte(p[1]) // G
			buf.WriteByte(p[2]) // R
			buf.WriteByte(p[3]) // A
		}
	}

	// AND mask (all zeros = fully opaque, we use alpha channel instead)
	andMask := make([]byte, andMaskSize)
	buf.Write(andMask)

	return buf.Bytes()
}
