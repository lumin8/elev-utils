package srtm

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
)

// SrtmTile holds file path and details of a single SRTM file (...which are themselves 'Tiles')
type SrtmTile struct {
	Latitude   int
	Longitude  int
	Name       string
	Dir        string
	Path       string
	SquareSize int
	Size       int64
}

// getElevation is main handler for a single lat lon input
func ElevationFromLatLon(demdir string, lat, lon float64) (float64, error) {
	srtm, err := getSrtm(demdir, lat, lon)
	if err != nil {
		return 0.0, err
	}

	elevation, err := srtm.getElevationFromSrtm(lat, lon)
	if err != nil {
		return 0.0, err
	}

	return elevation, nil
}

// getElevationFromWKT is not implemented yet

// getElevationFromBBOX is not implemented yet

// getSrtm is a specific handler for filling in details of a single SRTM Tile
func getSrtm(demdir string, lat, lon float64) (SrtmTile, error) {
	var srtm SrtmTile

	srtm.Dir = demdir

	srtm.getSrtmFileName(lat, lon)

	err := srtm.getSquareSize()
	if err != nil {
		return srtm, err
	}

	return srtm, nil
}

// getElevationFromSrtm is a specific handler for elevation, if SRTM details are known
func (s *SrtmTile) getElevationFromSrtm(lat, lon float64) (float64, error) {
	row, column := s.getRowAndColumn(lat, lon)

	elevation, err := s.getElevationFromRowAndColumn(row, column)
	if err != nil {
		return 0.0, err
	}

	return elevation, nil
}

// SRTM compliance prescribes distinct filenames eg. S56W072.hgt
func (s *SrtmTile) getSrtmFileName(lat, lon float64) {
	y := "S"
	if lat >= 0 {
		y = "N"
	}

	x := "W"
	if lon >= 0 {
		x = "E"
	}

	s.Latitude = int(math.Abs(math.Floor(lat)))
	s.Longitude = int(math.Abs(math.Floor(lon)))

	s.Name = fmt.Sprintf("%s%02d%s%03d.hgt", y, s.Latitude, x, s.Longitude)

	s.Path = filepath.Join(s.Dir, s.Name)
}

// the SquareSize determines the density of integers from the hgt file
// Each 3-arc-second data tile has 1442401 integers representing a 1201×1201 grid
// Each 1-arc-second data tile has 12967201 integers representing a 3601×3601 grid
func (s *SrtmTile) getSquareSize() error {

	// prepare file for observation
	f, err := os.Stat(s.Path)
	if err != nil {
		return err
	}

	// get the size
	s.Size = f.Size()

	// get the tile size
	if s.SquareSize <= 0 {
		squareSizeFloat := math.Sqrt(float64(s.Size) / 2.0)
		s.SquareSize = int(squareSizeFloat)

		if squareSizeFloat != float64(s.SquareSize) || s.SquareSize <= 0 {
			return errors.New(fmt.Sprintf("Invalid size for file %s: %d", s.Name, s.Size))
		}
	}

	return nil
}

// getRowAndColumn calculates the lookup []byte in the grid
// NOTE: row and column are int, therefore become FLOOR rounded values
func (s *SrtmTile) getRowAndColumn(lat, lon float64) (int, int) {
	var row, column int

	if lat >= 0 {
		row = int((float64(s.Latitude) + 1.0 - math.Abs(lat)) * (float64(s.SquareSize - 1.0)))
	} else {
		row = int((math.Abs(lat) - (float64(s.Latitude) - 1)) * (float64(s.SquareSize - 1.0)))
	}

	if lon >= 0 {
		column = int((lon - float64(s.Longitude)) * (float64(s.SquareSize - 1.0)))
	} else {
		column = int((float64(s.Longitude) - math.Abs(lon)) * (float64(s.SquareSize - 1.0)))
	}

	return row, column
}

// find the elevation value associated with the row and column
func (s *SrtmTile) getElevationFromRowAndColumn(row, column int) (float64, error) {
	i := int64(row*s.SquareSize + column)

	// calculate the byte range
	byteLocation := i * 2

	// open the file for reading
	f, err := os.Open(s.Path)
	if err != nil {
		return 0.0, err
	}
	defer f.Close()

	// get the results from the byte location
	_, err = f.Seek(byteLocation, 0)
	if err != nil {
		return 0.0, err
	}

	bytes := make([]byte, 2)
	response, err := io.ReadAtLeast(f, bytes, 2)
	if err != nil {
		return 0.0, err
	}
	result := bytes[:response]

	if len(result) != 2 {
		return 0.0, fmt.Errorf("Result []byte from strm file is too small: %v", result)
	}

	// do some magic
	// github.com/tkrajina/go-elevations/blob/master/geoelevations/srtm.go
	final := int(result[0])*256 + int(result[1])

	if final > 9000 {
		return 0.0, errors.New("result elevation is non logical")
	}

	return float64(final), nil
}
