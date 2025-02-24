package main

import (
	"encoding/json"
	"fmt"
	"../../elev-utils"
)

const (
	// NW Quadrant (Bozeman) z = ~1530m
	lat	= 45.638347
	lon	= -111.025257
	demdir	= "/data/dem/hdt/"
	floatpolygonstr = "[ [ [ -111.031436920166016, 45.643028055804137 ], [ -111.031651496887221, 45.632901041041464 ], [ -111.01572990417479, 45.632871032352256 ], [ -111.016137599945083, 45.643193073481086 ], [ -111.031436920166016, 45.643028055804137 ] ] ]"
)

func main() {

        // TEST #1, retrieve elevation for single point
        z, err := srtm.ElevationFromLatLon(lat,lon)
        if err != nil {
                fmt.Printf("%s",err.Error())
        }

        fmt.Printf("\n--Test #1 ElevationFromLatLon--\n")
	fmt.Printf("The elevation of Bozeman is: %v meters\n",z)

	// TEST #2, retreive pointcloud from bbox
	var bbox map[string]float64
	bbox = make(map[string]float64)
	bbox["lx"] = -111.031651496887221
        bbox["rx"] = -111.01572990417479
        bbox["ly"] = 45.632871032352256
        bbox["uy"] = 45.643193073481086

	fmt.Printf("\n--Test #2 ElevationFromBBOX--\n")

	boxptcloud, err := srtm.ElevationFromBBOX(bbox)
        if err != nil {
                fmt.Printf("%s",err.Error())
        }

	fmt.Printf("Number of bbox elevation 3D coords: %v\n",len(boxptcloud))
	fmt.Printf("Sample of first bbox 3D coord: %v\n",boxptcloud[0])
	fmt.Printf("Sample of last bbox 3D coord: %v\n",boxptcloud[len(boxptcloud)-1])

	// TEST #3, retrieve 3D pointcloud from float polygon
	var floatpolygon [][][]float64
	json.Unmarshal([]byte(floatpolygonstr), &floatpolygon)

	fmt.Printf("\n--Test #3 ElevationFromPolygon (poly as float)--\n")

	polyptcloud, err := srtm.ElevationFromPolygon(floatpolygon)
        if err != nil {
                fmt.Printf("%s",err.Error())
        }

	fmt.Printf("Number of polygon elevation 3D coords: %v\n",len(polyptcloud))
	fmt.Printf("Sample of first polygon contains 3D coord: %v\n",polyptcloud[0])
	fmt.Printf("Sample of last polygon contains 3D coord: %v\n",polyptcloud[len(polyptcloud)-1])
}
