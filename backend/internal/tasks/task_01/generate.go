package task_01

import (
	"fmt"
	"math/rand"
)

// Generate constructs only the fixed-seed dataset for this task.
func Generate() (dataset []string) {
	rng := rand.New(rand.NewSource(20261109))
	var records []string
	paths := []string{"/login", "/items", "/about", "/api/profile"}
	statuses := []int{200, 200, 401, 404, 500}
	for i := 0; i < 600; i++ {
		ip := fmt.Sprintf("10.20.%d.%d", i/250, i%250+1)
		burst := []int{threshold - 1, threshold, threshold + 1, 2*threshold + 2}[i%4]
		for second, count := range []int{burst, threshold - 3, threshold - 2, threshold - 1} {
			for j := 0; j < count; j++ {
				records = append(records, fmt.Sprintf("%s - - [10/May/2026:11:02:%02d +0000] \"GET %s HTTP/1.1\" %d", ip, second, paths[rng.Intn(len(paths))], statuses[rng.Intn(len(statuses))]))
			}
		}
	}
	rng.Shuffle(len(records), func(i, j int) { records[i], records[j] = records[j], records[i] })
	for i, record := range records {
		if i%97 == 0 {
			dataset = append(dataset, []string{"# rotated log fragment", "BROKEN RECORD", ""}[rng.Intn(3)])
		}
		dataset = append(dataset, record)
	}
	return dataset
}
