// Merges a set of images into a single "average" image.
//
// Details:
//   The algorithm that runs this tool combines all input images together on a
//   pixel-by-pixel basis, removing all pixels that have any of their R,G,B
//   channels at least N standard deviations away from the mean {R,G,B} value.
//   The average of the remaining pixels is used to set the output pixel's color.
//
// Usage:
//   go run main.go \
//    --path=Demo/Input/*.jpeg \
//    --output=Demo/output.jpeg
package main

import (
	"flag"
	"fmt"
	"image"
	"log"
	"os"
	"path/filepath"

	"github.com/montanaflynn/stats"

	"image/color"
	"image/jpeg"
)

var pathFlag = flag.String("path", "", "Path to files which supports glob formatting. Ex: 'Captchas/*.jpeg'.")
var outFlag = flag.String("output", "", "Name of the output file. Must end in '.jpeg'.")
var nFlag = flag.Float64("N", 1.3, "Strength of the pixel rejection, measured in multiples of standard deviation.")

func main() {
	flag.Parse()

	paths, err := filepath.Glob(*pathFlag)
	if err != nil {
		log.Fatalf("failed to parse path: %v", err)
	}
	if len(paths) == 0 {
		log.Fatalf("no files found for path: %v", *pathFlag)
	}

	images := []image.Image{}
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			log.Fatalf("failed opening %v: %v", p, err)
		}
		defer f.Close()

		i, _, err := image.Decode(f)
		if err != nil {
			log.Fatalf("failed decoding image %v: %v", f, err)
		}
		images = append(images, i)
	}

	bounds := images[0].Bounds()
	for _, i := range images {
		if i.Bounds() != bounds {
			log.Fatalf("unsupported operation; cannot merge images of different sizes: %v, %v", i.Bounds(), bounds)
		}
	}
	out := image.NewRGBA(image.Rectangle{bounds.Min, bounds.Max})

	// An image's bounds do not necessarily start at (0, 0), so the two loops start
	// at bounds.Min.Y and bounds.Min.X. Looping over Y first and X second is more
	// likely to result in better memory access patterns than X first and Y second.
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			colors := colors(x, y, images)
			c, err := meanColor(colors)
			if err != nil {
				log.Fatalf("failed to get mean pixel color at x=%v y=%v: %v", x, y, err)
			}
			out.Set(x, y, c)
		}
	}

	f, err := os.Create(*outFlag)
	if err != nil {
		log.Fatalf("failed to create output file %v: %v", *outFlag, err)
	}
	defer f.Close()

	err = jpeg.Encode(f, out, &jpeg.Options{Quality: 100})
	if err != nil {
		log.Fatalf("failed to save image to output file %v: %v", *outFlag, err)
	}
}

func colors(x, y int, images []image.Image) []color.Color {
	out := []color.Color{}
	for _, i := range images {
		out = append(out, i.At(x, y))
	}
	return out
}

func meanColor(colors []color.Color) (color.Color, error) {
	// Store RGBA data into a master slice of per-channel slices.
	// The index of the master has R=0, G=1, B=2, A=3
	channels := [][]float64{}
	var rs, gs, bs, as []float64
	for _, c := range colors {
		r, g, b, a := c.RGBA()
		rs = append(rs, float64(r))
		gs = append(gs, float64(g))
		bs = append(bs, float64(b))
		as = append(as, float64(a))
	}
	channels = append(channels, rs, gs, bs, as)

	// Convert RGBA slices into Mean and Std slices using the
	// same index scheme as before.
	means := []float64{}
	stddevs := []float64{}
	for _, c := range channels {
		m, err := stats.Mean(c)
		if err != nil {
			return nil, fmt.Errorf("failed to compute mean for %v: %v", c, err)
		}
		s, err := stats.StandardDeviationSample(c)
		if err != nil {
			return nil, fmt.Errorf("failed to compute sample standard deviation %v: %v", c, err)
		}

		means = append(means, m)
		stddevs = append(stddevs, s)
	}

	// Filter pixels that have a channel outside of N standard deviations
	var rsFilt, gsFilt, bsFilt, asFilt []float64
	for _, c := range colors {
		r, g, b, a := c.RGBA()
		N := *nFlag
		if float64(r) > means[0]+N*stddevs[0] || float64(r) < means[0]-N*stddevs[0] {
			continue
		}
		if float64(g) > means[1]+N*stddevs[1] || float64(g) < means[1]-N*stddevs[1] {
			continue
		}
		if float64(b) > means[2]+N*stddevs[2] || float64(b) < means[2]-N*stddevs[2] {
			continue
		}
		if float64(a) > means[3]+N*stddevs[3] || float64(a) < means[3]-N*stddevs[3] {
			continue
		}
		rsFilt = append(rsFilt, float64(r))
		gsFilt = append(gsFilt, float64(g))
		bsFilt = append(bsFilt, float64(b))
		asFilt = append(asFilt, float64(a))
	}
	if len(rsFilt) == 0 || len(gsFilt) == 0 || len(bsFilt) == 0 || len(asFilt) == 0 {
		return nil, fmt.Errorf("standard deviation filter removed all pixels; use a higher --N value to make the filter more permissive")
	}

	rMean, err := stats.Mean(rsFilt)
	if err != nil {
		return nil, fmt.Errorf("failed to compute red output using pixels %v: %v", rsFilt, err)
	}
	gMean, err := stats.Mean(gsFilt)
	if err != nil {
		return nil, fmt.Errorf("failed to compute green output using pixels %v: %v", gsFilt, err)
	}
	bMean, err := stats.Mean(bsFilt)
	if err != nil {
		return nil, fmt.Errorf("failed to compute blue output using pixels %v: %v", bsFilt, err)
	}
	aMean, err := stats.Mean(asFilt)
	if err != nil {
		return nil, fmt.Errorf("failed to compute alpha output using pixels %v: %v", asFilt, err)
	}
	return color.RGBA64{uint16(rMean), uint16(gMean), uint16(bMean), uint16(aMean)}, nil
}
