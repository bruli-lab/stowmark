package snapshot

import (
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
)

var ErrInvalidDataSubset = errors.New("invalid data subset: use a percentage such as 10% or a part such as 2/7")

type DataSubset struct {
	percent     float64
	part, total uint64
}

func ParseDataSubset(value string) (*DataSubset, error) {
	if number, ok := strings.CutSuffix(value, "%"); ok {
		percent, err := strconv.ParseFloat(number, 64)
		if err != nil || percent <= 0 || percent > 100 || math.IsNaN(percent) {
			return nil, fmt.Errorf("%w: %q", ErrInvalidDataSubset, value)
		}
		return &DataSubset{percent: percent}, nil
	}

	partText, totalText, ok := strings.Cut(value, "/")
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrInvalidDataSubset, value)
	}
	part, partErr := strconv.ParseUint(partText, 10, 64)
	total, totalErr := strconv.ParseUint(totalText, 10, 64)
	if partErr != nil || totalErr != nil || part == 0 || part > total {
		return nil, fmt.Errorf("%w: %q", ErrInvalidDataSubset, value)
	}
	return &DataSubset{part: part, total: total}, nil
}

func (d *DataSubset) String() string {
	if d.total > 0 {
		return fmt.Sprintf("%d/%d", d.part, d.total)
	}
	return strconv.FormatFloat(d.percent, 'f', -1, 64) + "%"
}

func (d *DataSubset) Select(hashes []string, random *rand.Rand) []string {
	if d.total > 0 {
		return d.selectPart(hashes)
	}
	count := int(math.Ceil(float64(len(hashes)) * d.percent / 100))
	selected := make([]string, 0, count)
	for _, index := range random.Perm(len(hashes))[:count] {
		selected = append(selected, hashes[index])
	}
	slices.Sort(selected)
	return selected
}

func (d *DataSubset) selectPart(hashes []string) []string {
	selected := make([]string, 0)
	for _, hash := range hashes {
		bucket := fnv.New64a()
		_, _ = bucket.Write([]byte(hash))
		if bucket.Sum64()%d.total == d.part-1 {
			selected = append(selected, hash)
		}
	}
	return selected
}
