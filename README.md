# SRTM: Elevation Utilities
This package contains several utilities that take a lat / lon value and returns the associated elevation *in meters_* anywhere in the world.

Most of these functions are designed specifically as an interface between SRTM data and a lat/lon query, reducing the need to rely on some other non-golang software (eg. shelling a call to gdal with `gdallocationinfo -valonly /path/to/dem.vrt -geoloc lat lon`).

Currently, the primary function is `srtm.ElevationFromLatLong`, which performs as you'd expect.  Also supported is `srtm.ElevationFromBBOX`and `srtm.ElevationFromPolygon`, which are nice to grab elevation points for areas.

The primary use case for this utility is to provide a very fast service for making multiple repeat elevation queries at scale, eg. finding elevation values for coordinates forming point/line/polygon geometries... or millions of them.

An example use of this code is to first establish one or more SRTMs' details by calling `getSrtm` for a series of lat/lons that cover the area of investigation (a single 1-arc-second SRTM tile is good for about 20-30m square), then fetching elevation for *n* coordinates.

This application does not require that BBOX or Polygon fit within support srtm tiles, neighboring tiles will be grabbed and searched for appropriate values.

Early versions of this work (pre 2025) converts the lat lon to a byte lookup value from within a file stored locally.  
2025 + version of this software requests the appropriate srtm tile(s) from a google storage bucket, loads into into memory, and performs the requests.  The former process is very fast, the latter only slowed by the egress speeds from the bucket (a few seconds/tile).

For more information on SRTM, please visit `https://wiki.openstreetmap.org/wiki/SRTM`.

Currently, the author maintains a private worldwide set of SRTM data on a private google storage bucket.  This set may be made available on a per-inquiry basis.


### TBD
Triangulate the appropriate height of a point that falls between the vales surrounding a given point.  Currently, errors exists where a lat lon lookup results in an elevation that is too high or too low because it was drawn from the point nearest in the direction toward 0,0 (eastern atlantic at the equator)... eg, 45.123456 |  -111.123456  actually looks up the value for 45.123  |  -111.123, as much as 30 meters away for 1-arc second data, or 90 meters away for 3-arc second data _at the equator_.   The data density of these grids is even less at the poles, where a 3-arc second grid is spaced out up to 90 meters in a N-S direction.

Therefore, averaging of the surrounding points at least provides a step forward in approximately true elevation at a given point.  Best yet, is to construct denser elevation data.


### Notes
1) Several functions in the **SRTM** package were adapted from `https://github.com/tkrajina/go-elevations`, whose repo is designed as a standalone http client service that utilizes SRTM data available online... reading and storing the SRTM tile into memory... useful for maintaining a small footprint but is not applicable for coordinate lookups worldwide @ scale.

2) This package requires that 1-arc-second or 3-arc-second SRTM data are already available in a directory accessible by the code that calls this package.  If the data is not present, the response will be an error of "SRTM not available, please download" or similar.

3) No efforts are planned to keep this repo backwards-compatible.
