package main

import (
	"fmt"
	"regexp"
	"strings"
)

// BuildTournamentName constructs a tournament name from its ID
func BuildTournamentName(tournamentID string) string {
	// Split the ID into region and code parts
	parts := strings.SplitN(tournamentID, "-", 2)
	if len(parts) < 2 {
		return fmt.Sprintf("Tournament %s", tournamentID)
	}

	region := parts[0]
	code := parts[1]

	// Map for region codes
	regionMap := map[string]string{
		"kr":   "Korea",
		"eu":   "Europe",
		"na":   "North America",
		"sa":   "South America",
		"cn":   "China",
		"sea":  "Southeast Asia",
		"as":   "Asia",
		"tw":   "Taiwan",
		"jp":   "Japan",
		"oc":   "Oceania",
		"am":   "Americas",
		"ap":   "Asia Pacific",
		"main": "Main",
	}

	// Extract tournament series and phase
	seriesRegex := regexp.MustCompile(`^([a-z]+)([0-9]*)(.*)$`)
	matches := seriesRegex.FindStringSubmatch(code)

	if len(matches) < 4 {
		return fmt.Sprintf("%s Tournament %s", getRegionName(region, regionMap), code)
	}

	series := matches[1]
	season := matches[2]
	phase := matches[3]

	// Map for tournament series
	seriesMap := map[string]string{
		"pcs":  "PUBG Continental Series",
		"pgc":  "PUBG Global Championship",
		"pml":  "PUBG Mobile League",
		"pws":  "PUBG Weekly Series",
		"bsc":  "PUBG Battle Showcase",
		"gth":  "Global Tournament Hub",
		"pas":  "PUBG Americas Series",
		"pgs":  "PUBG Global Series",
		"pec":  "PUBG European Championship",
		"pnc":  "PUBG Nations Cup",
		"pvs":  "PUBG Vietnam Series",
		"mt":   "Master Tournament",
		"race": "PUBG Race",
		"pcl":  "PUBG Champions League",
		"ptc":  "PUBG Thailand Championship",
		"pts":  "PUBG Thailand Series",
		"pkl":  "PUBG Korea League",
		"pkp":  "PUBG Korea Professional",
		"pls":  "PUBG Local Series",
		"ptm":  "PUBG Team Match",
		"pp":   "PUBG Professional",
		"pc":   "PUBG Championship",
		"ppc":  "PUBG Pro Circuit",
		"fth":  "Fall Tournament Hub",
		"apl":  "Asia PUBG League",
		"pjs":  "PUBG Japan Series",
		"leo":  "League of Origin",
	}

	// Map for phase codes
	phaseMap := map[string]string{
		"gf":   "Grand Finals",
		"lc":   "Last Chance Qualifier",
		"oq":   "Open Qualifier",
		"gs":   "Group Stage",
		"wb":   "Winners Bracket",
		"lb":   "Losers Bracket",
		"qf":   "Quarter Finals",
		"sf":   "Semi Finals",
		"w1":   "Week 1",
		"w2":   "Week 2",
		"w3":   "Week 3",
		"w4":   "Week 4",
		"w5":   "Week 5",
		"f":    "Finals",
		"s1":   "Season 1",
		"s2":   "Season 2",
		"p1":   "Phase 1",
		"p2":   "Phase 2",
		"c1":   "Cycle 1",
		"c2":   "Cycle 2",
		"c3":   "Cycle 3",
		"po":   "Playoffs",
		"q":    "Qualifiers",
		"rp":   "Regional Playoffs",
		"tm":   "Tournament",
		"fs":   "Finals Stage",
		"ms":   "Main Stage",
		"api":  "API Tournament",
		"test": "Test Tournament",
		"t":    "Tournament",
	}

	// Extract more detailed phase info (for phases like w1, s2, etc.)
	phaseCode := ""
	phaseDetail := phase

	if phase != "" {
		phaseRegex := regexp.MustCompile(`^([a-z]+)(.*)$`)
		phaseMatches := phaseRegex.FindStringSubmatch(phase)
		if len(phaseMatches) > 2 {
			phaseCode = phaseMatches[1]
			phaseDetail = phaseMatches[2]
		}
	}

	// Build the tournament name
	regionName := getRegionName(region, regionMap)
	seriesName := getSeriesName(series, seriesMap)
	phaseName := getPhaseName(phaseCode, phaseDetail, phaseMap)

	// Put it all together with proper formatting
	tournamentName := seriesName

	// Add season if available
	if season != "" {
		tournamentName += " " + season
	}

	// Add phase if available
	if phaseName != "" {
		tournamentName += " - " + phaseName
	}

	// Add region at the beginning if not already part of the series name
	if !strings.Contains(strings.ToLower(seriesName), strings.ToLower(regionName)) && regionName != "Unknown" {
		tournamentName = regionName + " " + tournamentName
	}

	return tournamentName
}

// Helper function to get region name
func getRegionName(region string, regionMap map[string]string) string {
	if name, ok := regionMap[region]; ok {
		return name
	}
	return "Unknown"
}

// Helper function to get series name
func getSeriesName(series string, seriesMap map[string]string) string {
	if name, ok := seriesMap[series]; ok {
		return name
	}
	return strings.ToUpper(series) + " Tournament"
}

// Helper function to get phase name
func getPhaseName(phaseCode string, phaseDetail string, phaseMap map[string]string) string {
	if name, ok := phaseMap[phaseCode]; ok {
		// For cases like w1, w2, s1, etc.
		if phaseDetail != "" {
			// Try to match detailed phase info
			if detailName, ok := phaseMap[phaseCode+phaseDetail]; ok {
				return detailName
			}
			return name
		}
		return name
	} else if phaseCode != "" {
		return strings.ToUpper(phaseCode) + " " + phaseDetail
	}
	return ""
}

func main() {
	// Example usage
	examples := []string{
		"kr-pws143",
		"sea-mt1",
		"kr-ppc13",
		"eu-pecs25",
		"na-pas5oq",
		"as-pgc24gf",
		"sea-gth6",
		"main-krt",
		"cn-pcls25",
		"kr-race2o",
		"jp-pjc24qf",
	}

	for _, id := range examples {
		fmt.Printf("ID: %s -> Name: %s\n", id, BuildTournamentName(id))
	}
}
