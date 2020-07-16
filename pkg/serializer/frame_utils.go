package serializer

import "io"

// FrameList is a list of frames (byte arrays), used for convenience functions
type FrameList [][]byte

// ReadFrameList is a convenience method that reads all available frames from the FrameReader
// into a returned FrameList
func ReadFrameList(fr FrameReader) (FrameList, error) {
	// TODO: Create an unit test for this function
	var frameList [][]byte
	for {
		// Read until we get io.EOF or an error
		frame, err := fr.ReadFrame()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		// Append all frames to the returned list
		frameList = append(frameList, frame)
	}
	return frameList, nil
}

// WriteFrameList is a convenience method that writes a set of frames to a FrameWriter
func WriteFrameList(fw FrameWriter, frameList FrameList) error {
	// TODO: Create an unit test for this function
	// Loop all frames in the list, and write them individually to the FrameWriter
	for _, frame := range frameList {
		if _, err := fw.Write(frame); err != nil {
			return err
		}
	}
	return nil
}
