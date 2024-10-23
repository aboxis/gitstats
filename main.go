package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Based on
// git --no-pager log --pretty="%an" --shortstat --since="2024-03-01" --until="2024-03-31"

type ChangesStats struct {
	Insertions int
	Deletions  int
}

type GlobalStats struct {
	Stats           map[string]map[string]ChangesStats
	totalInsertions int
	totalDeletions  int
}

func main() {
	var gb GlobalStats
	gb.Stats = make(map[string]map[string]ChangesStats)

	insertionRegex := regexp.MustCompile(`(\d+) insertions?\(\+\)`)
	deletionRegex := regexp.MustCompile(`(\d+) deletions?\(-\)`)

	monthsBackPtr := flag.Int("m", 1, "Number of months to check backward")
	allReposPtr := flag.Bool("a", false, "Analyze all repositories in subdirectories")
	baseDirStr := flag.String("p", ".", "Path for analysis ( . by default)")
	flag.Parse()

	baseDir := *baseDirStr

	for i := 0; i < *monthsBackPtr; i++ {
		// Calculate the date range

		year, month, _ := time.Now().AddDate(0, -i, 0).Date()
		firstDayOfMonth := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
		lastDayOfMonth := firstDayOfMonth.AddDate(0, 1, -1)

		//globalStats := make(map[string][2]int) // Global stats across all repos

		processDir := func(dir string) error {
			args := []string{"--no-pager", "-C", dir, "log", "--pretty=%ae", "--shortstat",
				"--since=" + firstDayOfMonth.Format("2006-01-02"),
				"--until=" + lastDayOfMonth.Format("2006-01-02"),
				"--", "*.swift",
				"--", "*.yml",
				"--", "*.java",
				"--", "*.kt",
				"--", "*.md",
				"--", "*.php",
			}

			commandStr := strings.Join(args, " ")
			log.Println(commandStr)

			cmd := exec.Command("git", args...)
			output, err := cmd.Output()
			if err != nil {
				return fmt.Errorf("failed to execute command: %s", err)
			}

			lines := strings.Split(string(output), "\n")
			author := ""
			stats := make(map[string][2]int) // [0]: insertions, [1]: deletions

			for _, line := range lines {
				if line == "" {
					continue
				}

				if strings.Contains(line, "files changed") ||
					strings.Contains(line, "file changed") {
					insertions := insertionRegex.FindStringSubmatch(line)
					deletions := deletionRegex.FindStringSubmatch(line)

					var ins, del int
					if len(insertions) > 0 {
						fmt.Sscanf(insertions[1], "%d", &ins)
					}
					if len(deletions) > 0 {
						fmt.Sscanf(deletions[1], "%d", &del)
					}

					userStats := stats[author]
					userStats[0] += ins
					userStats[1] += del
					stats[author] = userStats
					gb.totalInsertions += ins
					gb.totalDeletions += del

				} else {
					author = line // Assuming every non-empty line that's not stats is an author
				}
			}

			// Accumulate global stats
			for author, counts := range stats {
				if _, exists := gb.Stats[author]; !exists {
					gb.Stats[author] = make(map[string]ChangesStats)
				}
				monthStr := firstDayOfMonth.Format("(2006-01) January 2006")
				authorMonthStats := gb.Stats[author][monthStr]
				authorMonthStats.Insertions += counts[0]
				authorMonthStats.Deletions += counts[1]
				gb.Stats[author][monthStr] = authorMonthStats
			}

			return nil
		}

		if *allReposPtr {
			dirs, err := os.ReadDir(baseDir)
			if err != nil {
				fmt.Printf("Failed to read directory: %s\n", err)
				return
			}

			for _, dir := range dirs {
				if dir.IsDir() {
					dirPath := filepath.Join(baseDir, dir.Name())
					if err := processDir(dirPath); err != nil {
						fmt.Println(err)
					}
				}
			}
		} else {
			if err := processDir(baseDir); err != nil {
				fmt.Println(err)
				return
			}
		}
	}
	printStats(gb)

}

func printStats(globalStats GlobalStats) {
	//red := "\033[31m"
	green := "\033[32m"
	yellow := "\033[33m"
	blue := "\033[94m"
	reset := "\033[0m"

	if len(globalStats.Stats) == 0 {
		return
	}
	uniqueMonths := make(map[string]bool)
	for _, months := range globalStats.Stats {
		for month := range months {
			uniqueMonths[month] = true
		}
	}
	var monthsOrdered []string
	for month := range uniqueMonths {
		monthsOrdered = append(monthsOrdered, month)
	}
	sort.Strings(monthsOrdered) // Sort the months if needed

	// Step 3: Aggregate and print data per month
	for _, month := range monthsOrdered {
		fmt.Printf("-----------------------------\n")
		fmt.Printf("%s%s%s\n", yellow, month, reset)
		totalInsertions := 0
		totalDeletions := 0

		// Prepare a slice for sorting by insertions
		type authorStats struct {
			Author     string
			Insertions int
		}
		var monthStats []authorStats
		for author, monthsStats := range globalStats.Stats {
			if stats, exists := monthsStats[month]; exists {
				monthStats = append(monthStats, authorStats{Author: author, Insertions: stats.Insertions})
				totalInsertions += stats.Insertions
				totalDeletions += stats.Deletions
			}
		}

		// Sort the slice by insertions in descending order
		sort.Slice(monthStats, func(i, j int) bool {
			return monthStats[i].Insertions > monthStats[j].Insertions
		})

		// Print sorted stats for the month
		for _, stats := range monthStats {
			fmt.Printf("  %-30s %s%5d%s lines\n", stats.Author, green, stats.Insertions, reset)
		}

		fmt.Printf("%sSummary:%s %s%d%s %stotal lines%s\n", yellow, reset,
			green, totalInsertions, reset, yellow, reset)
	}
	fmt.Printf("\n%s-----------------------------%s\n", blue, reset)

	// Aggregate total insertions by author
	authorInsertions := make(map[string]int)
	for author, months := range globalStats.Stats {
		for _, stats := range months {
			authorInsertions[author] += stats.Insertions
		}
	}

	// Convert map to slice of pairs for sorting
	type kv struct {
		Author     string
		Insertions int
	}
	var sortedAuthors []kv
	for author, ins := range authorInsertions {
		sortedAuthors = append(sortedAuthors, kv{author, ins})
	}

	// Sort authors by total insertions in descending order
	sort.Slice(sortedAuthors, func(i, j int) bool {
		return sortedAuthors[i].Insertions > sortedAuthors[j].Insertions
	})

	// Print the sorted summary of insertions by developers
	fmt.Printf("%sTotal lines by developer:%s\n", blue, reset)
	for _, kv := range sortedAuthors {
		fmt.Printf("  %-30s %s%5d%s lines\n", kv.Author, green, kv.Insertions, reset)
	}

	fmt.Printf("%s-----------------------------%s\n", blue, reset)
	fmt.Printf("Total summary: %s%d%s total lines\n",
		green, globalStats.totalInsertions, reset)
}
