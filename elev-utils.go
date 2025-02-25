package srtm

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/bobg/gcsobj"
	geo "github.com/paulmach/go.geo"
	"github.com/paulmach/orb"
	"github.com/paulmach/orb/planar"
)

const (
	// this package links to the google storage bucket DEM by default
	bucket_name = "dem-data"
	demvrt      = "dem/hdt/earthdem.vrt"
	demdir      = "dem/hdt/"

	// select the spacing in lat/lon decimal degrees of the x/y elevation points
	step  = .0003 //about 20 meters at 45 deg north)
	width = .01   //about 3km using width as diameter
)

var (
	// initialize a project wide scope client request
	cli storage.Client

	// create a project-wide accessible temp data storage
	srtmTiles = make(map[string]srtmTile)
)

// SrtmTile holds file path and details of a single SRTM request
type srtmRequest struct {
	Latitude   int
	Longitude  int
	Name       string
	Dir        string
	Path       string
	SquareSize int
	Size       int64
}

// srtmTile holds the data for each tile
type srtmTile struct {
	Name       string
	Dir        string
	Path       string
	SquareSize int
	Size       int64
	Data       []byte
}

// create Google Storage client (cli) and bucket (bkt)
func init() {
	ctx := context.Background()
	cli, err := storage.NewClient(ctx)
	if err != nil {
		fmt.Println("Fata error: %s", err.Error())
	}
	defer cli.Close()
}

// ElevationFromLatLon is main handler for a single lat lon input
func ElevationFromLatLon(lat, lon float64) (float64, error) {
	srtm, err := makeSrtm(lat, lon)
	if err != nil {
		return math.NaN(), err
	}

	elevation, err := srtm.getElevationFromSrtm(lat, lon)
	if err != nil {
		return elevation, err
	}

	return elevation, nil
}

// TBD AverageElevationFromLatLon creates a polygon surrounding the point, and maths a more approximate elevation
func AverageElevationFromLatLon(lat, lon float64) (float64, error) {
	// TBD triangulate from four? closest points
	return math.NaN(), nil
}

// ElevationFromBBOX parses each LatLon, grabs ALL points within enclosed extent
func ElevationFromBBOX(bbox map[string]float64) ([][]float64, error) {
	var x []float64
	var y []float64

	// get the min/max x, and min/max y values, to only the 5th digit
	for k, v := range bbox {
		v = math.Round(v*100000) / 100000
		if strings.Contains(strings.ToLower(k), "x") {
			x = append(x, v)
		} else {
			y = append(y, v)
		}
	}

	// sort ascending
	sort.Float64s(x)
	sort.Float64s(y)

	// build the range of points covered by the bbox
	if len(x) < 2 || len(y) < 2 {
		return nil, fmt.Errorf("bbox is malformed, can not complete request, hint: %v", bbox)
	}

	// the third digit in this call is incrementing by .001 [approx 3 arc seconds] )
	xrange := makeRange(x[0], x[1])
	yrange := makeRange(y[0], y[1])

	// combine all the possible x,y combinations
	var ptcloud [][]float64

	for _, lon := range xrange {
		lonRnd := float64(int(lon*10000000)) / 10000000

		for _, lat := range yrange {
			latRnd := float64(int(lat*10000000)) / 10000000

			// look up the elevation value for each point
			z, err := ElevationFromLatLon(latRnd, lonRnd)

			if err != nil {
				fmt.Errorf("Not Fatal: [ElevationFromLatLon] in [ElevationFromBBOX] %v", err)
			} else {
				// apend the elevation to the point
				//fmt.Printf("Appended point to cloud: %v", []float64{lon, lat, z})
				ptcloud = append(ptcloud, []float64{lonRnd, latRnd, z})
			}
		}
	}

	return ptcloud, nil

}

// ElevationFromPolygon parses the polygon (GEOJSON), returns point cloud of containing 3D points
func ElevationFromPolygon(polygon [][][]float64) ([][]float64, error) {
	// initiate the 2D point cloud early to capture all valid points
	var polycloud [][]float64

	// get bbox bounding extent of polygon
	var bbox map[string]float64
	bbox = make(map[string]float64)
	bbox["lx"] = 0
	bbox["rx"] = 0
	bbox["ly"] = 0
	bbox["uy"] = 0

	for _, feature := range polygon {

		for _, coord := range feature {

			// make sure projection is 4326!
			lonBig, latBig := To4326(coord[0], coord[1])

			lon := float64(int(lonBig*10000000)) / 10000000
			lat := float64(int(latBig*10000000)) / 10000000


			// also, make sure each poly point gets included in the elev lookup array
			z, err := ElevationFromLatLon(lat, lon)
			if err != nil {
				return nil, fmt.Errorf("Fatal: [ElevationFromBbox] in [ElevationFromPolygon] --> %v", err)
			} else {
				polycloud = append(polycloud, []float64{lon, lat, z})
			}

			// if the inbound X is outside of current extent, grow extent
			if lon < bbox["lx"] || bbox["lx"] == 0 {
				bbox["lx"] = lon
			}
			if lon > bbox["rx"] || bbox["rx"] == 0 {
				bbox["rx"] = lon
			}

			// if the inbound Y is outside of current extent, grow extent
			if lat < bbox["ly"] || bbox["ly"] == 0 {
				bbox["ly"] = lat
			}
			if lat > bbox["uy"] || bbox["uy"] == 0 {
				bbox["uy"] = lat
			}
		}

	}

	// get all elevations in bbox
	ptcloud, err := ElevationFromBBOX(bbox)
	if err != nil {
		return nil, fmt.Errorf("Fatal: [ElevationFromBbox] in [ElevationFromPolygon] --> %v", err)
	}

	// retain bbox points only within polygon boundary
	for _, pt := range ptcloud {
		if IsPointInsidePolygon(polygon, pt) == true {
			polycloud = append(polycloud, pt)
		}
	}

	return polycloud, nil

}

// IsPointInsidePolygon uses paulmach's orb.planar package to make bool determination of location
func IsPointInsidePolygon(feature [][][]float64, floatpt []float64) bool {
	// need test point to be of orb.Point type
	var testpoint orb.Point
	testpoint[0] = floatpt[0]
	testpoint[1] = floatpt[1]

	// need to parse each polygon as linestring (closed)
	for _, linestring := range feature {

		// orb.Planar requires an 'orb.Ring' struct
		var ring orb.Ring

		// add each of the features' points as orb.Point to orb.Linestring
		for _, pt := range linestring {
			pt := orb.Point{pt[0], pt[1]}
			ring = append(ring, pt)
		}

		// test if the ring encircles the test point
		if planar.RingContains(ring, testpoint) {
			return true
		}
	}

	return false
}

// IsPointInsideMultiPolygon recursively checks each inner polygon or hole
func IsPointInsideMultiPolygon(feature [][][][]float64, floatpt []float64) bool {
	for _, polygon := range feature {
		if IsPointInsidePolygon(polygon, floatpt) == true {
			return true
		}
	}

	return false
}

// makeSrtm is a specific handler for filling in details of a single SRTM tile and request
func makeSrtm(lat, lon float64) (srtmRequest, error) {
	var srtm srtmRequest

	srtm.Dir = demdir

	srtm.makeSrtmFileName(lat, lon)

	err := srtm.getSquareSize()
	if err != nil {
		return srtm, err
	}

	return srtm, nil
}

// SRTM compliance prescribes distinct filenames eg. S56W072.hgt
// ONLY CALLED FROM makeSrtm
func (self *srtmRequest) makeSrtmFileName(lat, lon float64) {
	y := "S"
	if lat >= 0 {
		y = "N"
	}

	x := "W"
	if lon >= 0 {
		x = "E"
	}

	self.Latitude = int(math.Abs(math.Floor(lat)))
	self.Longitude = int(math.Abs(math.Floor(lon)))

	self.Name = fmt.Sprintf("%s%02d%s%03d.hgt", y, self.Latitude, x, self.Longitude)

	self.Path = filepath.Join(self.Dir, self.Name)
}

// the SquareSize determines the density of integers from the hgt file
// Each 3-arc-second data tile has 1442401 integers representing a 1201×1201 grid
// Each 1-arc-second data tile has 12967201 integers representing a 3601×3601 grid
// ONLY CALLED FROM makeSrtm
func (self *srtmRequest) getSquareSize() error {

	// check to see if tile is already available
	_, ok := srtmTiles[self.Name]
	if ok {
		self.Size = srtmTiles[self.Name].Size
		self.SquareSize = srtmTiles[self.Name].SquareSize
		return nil
	}

	// otherwise, fetch remotely
	ctxy := context.Background()
	cli, err := storage.NewClient(ctxy)
	if err != nil {
		fmt.Println("Fatal error: %s", err.Error())
	}
	defer cli.Close()

	// Connect to the GS CLIENT FILE
	bkt := cli.Bucket(bucket_name)
	obj := bkt.Object(self.Path)
	stats, err := obj.Attrs(ctxy)
	if err != nil {
		return err
	}

	// get the size
	self.Size = stats.Size

	// get the tile size
	if self.SquareSize <= 0 {
		squareSizeFloat := math.Sqrt(float64(self.Size) / 2.0)
		self.SquareSize = int(squareSizeFloat)

		if squareSizeFloat != float64(self.SquareSize) || self.SquareSize <= 0 {
			return errors.New(fmt.Sprintf("Invalid size for file %s: %d", self.Name, self.Size))
		}
	}

	return nil
}

// getElevationFromSrtm is a specific handler for elevation
// calls row and column in place of lat and lon
func (self *srtmRequest) getElevationFromSrtm(lat, lon float64) (float64, error) {
	row, column := self.getRowAndColumn(lat, lon)

	elevation, err := self.getElevationFromRowAndColumn(row, column)
	if err != nil {
		return elevation, fmt.Errorf("elevation is %v for lat long of %v, %v", elevation, lat, lon)
	}

	return elevation, nil
}

// getRowAndColumn calculates the lookup []byte in the grid
// NOTE: row and column are int, therefore become FLOOR rounded values
func (self *srtmRequest) getRowAndColumn(lat, lon float64) (int, int) {
	var row, column int

	if lat >= 0 {
		row = int((float64(self.Latitude) + 1.0 - math.Abs(lat)) * (float64(self.SquareSize - 1.0)))
	} else {
		row = int((math.Abs(lat) - (float64(self.Latitude) - 1)) * (float64(self.SquareSize - 1.0)))
	}

	if lon >= 0 {
		column = int((lon - float64(self.Longitude)) * (float64(self.SquareSize - 1.0)))
	} else {
		column = int((float64(self.Longitude) - math.Abs(lon)) * (float64(self.SquareSize - 1.0)))
	}

	return row, column
}

// getElevationFromRowAndColumn reads file, grabs elev associated with the byte row and column
// This is the fxtn that actually does the elev lookup work; optimize HERE
func (self *srtmRequest) getElevationFromRowAndColumn(row, column int) (float64, error) {
	i := int64(row*self.SquareSize + column)

	// calculate the byte range
	byteLocation := i * 2
	byteLocationTo := byteLocation + 2

	// make sure the data is available locally
	err := fetchSrtmTile(self)
	if err != nil {
		return math.NaN(), err
	}

	// get the results from the byte location
	// Deprecated
	/*
		_, _ = f.Seek(byteLocation, 0)
		bytes := make([]byte, 2)
		response, _ := io.ReadAtLeast(f, bytes, 2)
		result := bytes[:response]
	*/
	//bytes := make([]byte, 2)
	//response, _ := io.ReadAtLeast(bytes, 2)

	result := srtmTiles[self.Name].Data[byteLocation:byteLocationTo]

	if len(result) != 2 {
		errstring := fmt.Sprintf("%v", result)
		err = errors.New("Result []byte from strm file is too small: " + errstring)
		return math.NaN(), err
	}

	// do some magic
	// github.com/tkrajina/go-elevations/blob/master/geoelevations/srtm.go
	final := int(result[0])*256 + int(result[1])

	if final > 11000 {
		err = errors.New("result elevation is non logical")
		return math.NaN(), fmt.Errorf("result is not logical, encountered float64 value of: %v", float64(final))
	}

	return float64(final), nil
}

// fetchSrtmTile collects the remote tile, adds to srtmTiles array
func fetchSrtmTile(self *srtmRequest) error {
	// check if tile already exists locally
	_, ok := srtmTiles[self.Name]
	if ok {
		return nil
	}

	// fetch if not
	// build google storage request
	ctxy := context.Background()
	cli, err := storage.NewClient(ctxy)
	bkt := cli.Bucket(bucket_name)
	obj := bkt.Object(self.Path)
	f, err := gcsobj.NewReader(ctxy, obj)
	if err != nil {
		return fmt.Errorf("elevation is not available for tile %v", self.Name)
	}
	defer f.Close()

	// consrtuct the tile
	var tile srtmTile
	tile.Name = self.Name
	tile.Path = self.Path
	tile.Dir = self.Dir
	tile.SquareSize = self.SquareSize
	tile.Size = self.Size
	data, err := ioutil.ReadAll(f)
	if err != nil {
		return fmt.Errorf("data was not read into local source tile %v", self.Name)
	}
	tile.Data = data

	srtmTiles[tile.Name] = tile

	return nil
}

// makeRange takes in a min and max value, and builds the range from there
func makeRange(min float64, max float64) []float64 {

	// build the range

	truestep := int((max-min)/step) + 1

	a := make([]float64, truestep)
	for i := range a {
		if i == 0 {
			a[i] = min
			continue
		}
		a[i] = a[i-1] + step
	}
	return a
}

//NewBBOX takes an X and Y and creates a bbox
func NewBBOX(x string, y string) map[string]float64 {
	var bbox map[string]float64
	bbox = make(map[string]float64)
	bbox["lx"] = Str2Fixed(x) - width
	bbox["rx"] = Str2Fixed(x) + width
	bbox["ly"] = Str2Fixed(y) - width
	bbox["uy"] = Str2Fixed(y) + width
	return bbox
}

// Str2Fixed is a little helper function taking a string value and returns a float64
func Str2Fixed(num string) float64 {
	val, _ := strconv.ParseFloat(num, 64)
	j := strconv.FormatFloat(val, 'f', 2, 64)
	k, _ := strconv.ParseFloat(j, 64)
	return k
}

// To4326 converts coordinates to EPSG:4326 projection
// if 4325 coordinates are passed in, does not affect the originals
func To4326(x float64, y float64) (float64, float64) {
	if x > 180 || x < -180 || y > 180 || y < -180 {
		mercPoint := geo.NewPoint(x, y)
		geo.Mercator.Inverse(mercPoint)
		x = math.Round(mercPoint[0]*10000) / 10000
		y = math.Round(mercPoint[1]*10000) / 10000
	}

	return x, y
}
