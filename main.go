package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Ability represents a single ability with its details
type Ability struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Image       string `json:"image"`
}

// Agent represents a Valorant agent with all its information
type Agent struct {
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	Abilities []Ability `json:"abilities"`
	Media     []string  `json:"media"`
	Markdown  string    `json:"markdown"`
}

// Agents holds all agents, keyed by agent name
type Agents struct {
	Agents map[string]Agent `json:"agents"`
}

// AgentFile represents the structure of the JSON files
type AgentFile struct {
	Markdown string                 `json:"markdown"`
	Metadata map[string]interface{} `json:"metadata"`
}

func main() {
	agents := Agents{
		Agents: make(map[string]Agent),
	}

	dir := "./test-data/agens"
	err := readAgentFiles(dir, &agents)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading agent files: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("found %d agents\n", len(agents.Agents))
	// Marshal to JSON and print to stdout
	// Use an encoder with HTML escaping disabled so & characters aren't escaped as \u0026
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(agents); err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		os.Exit(1)
	}
}

// readAgentFiles reads all JSON files from the directory and processes them
func readAgentFiles(dir string, agents *Agents) error {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(dir, file.Name())
		data, err := ioutil.ReadFile(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Error reading %s: %v\n", filePath, err)
			continue
		}

		var agentFile AgentFile
		if err := json.Unmarshal(data, &agentFile); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Error parsing JSON in %s: %v\n", filePath, err)
			continue
		}

		agent := parseMarkdown(agentFile.Markdown)
		if agent.Name != "" {
			agents.Agents[agent.Name] = agent
		}
	}

	return nil
}

// parseMarkdown extracts structured data from the markdown field
func parseMarkdown(markdown string) Agent {
	agent := Agent{
		Markdown: markdown,
	}

	// Extract name from # heading
	agent.Name = extractName(markdown)

	// Extract role
	agent.Role = extractRole(markdown)

	// Extract all image URLs for media
	agent.Media = extractImages(markdown)

	// Parse abilities
	agent.Abilities = parseAbilities(markdown)

	return agent
}

// extractName extracts the agent name from the first # heading
func extractName(markdown string) string {
	re := regexp.MustCompile(`^#\s+(.+)$`)
	lines := strings.Split(markdown, "\n")
	for _, line := range lines {
		if matches := re.FindStringSubmatch(line); matches != nil {
			return strings.TrimSpace(matches[1])
		}
	}
	return ""
}

// extractRole extracts the role from the ROLE section
func extractRole(markdown string) string {
	re := regexp.MustCompile(`(?m)^ROLE\s*\n\s*(.+)$`)
	if matches := re.FindStringSubmatch(markdown); matches != nil {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// extractImages extracts all image URLs from markdown and URL-encodes them
func extractImages(markdown string) []string {
	re := regexp.MustCompile(`!\[.*?\]\((.*?)\)`)
	matches := re.FindAllStringSubmatch(markdown, -1)

	images := make([]string, 0, len(matches))
	seen := make(map[string]bool)

	for _, match := range matches {
		if len(match) > 1 {
			rawURL := strings.TrimSpace(match[1])
			if rawURL != "" && !seen[rawURL] {
				// URL-encode the URL so it can be copied and pasted directly
				encodedURL := urlEncode(rawURL)
				images = append(images, encodedURL)
				seen[rawURL] = true
			}
		}
	}

	return images
}

// parseAbilities extracts abilities with their images, names, and descriptions
func parseAbilities(markdown string) []Ability {
	abilities := []Ability{}

	// Find the SPECIAL ABILITIES section
	abilitiesIndex := strings.Index(markdown, "## SPECIAL ABILITIES")
	if abilitiesIndex == -1 {
		return abilities
	}

	// Get the section after SPECIAL ABILITIES, but stop before footer content
	abilitiesSection := markdown[abilitiesIndex:]

	// Find where footer starts (common patterns)
	footerStart := len(abilitiesSection)
	footerPatterns := []string{
		"\n- [Download Riot Mobile Companion App]",
		"\n[Riot Games]",
		"\n© 2020-2025",
	}
	for _, pattern := range footerPatterns {
		if idx := strings.Index(abilitiesSection, pattern); idx != -1 && idx < footerStart {
			footerStart = idx
		}
	}
	abilitiesSection = abilitiesSection[:footerStart]

	// Extract numbered ability images (1-4)
	abilityImageRe := regexp.MustCompile(`(?m)^(\d+)\.\s*!\[.*?\]\((.*?)\)`)
	imageMatches := abilityImageRe.FindAllStringSubmatch(abilitiesSection, -1)

	// Map numbered images to their positions
	imageMap := make(map[int]string)
	for _, match := range imageMatches {
		if len(match) >= 3 {
			num := 0
			fmt.Sscanf(match[1], "%d", &num)
			imageMap[num] = match[2]
		}
	}

	// Find ability names (ALL CAPS, standalone lines, after the numbered images)
	// Pattern: After numbered images and a large image, we get ability names in ALL CAPS
	// followed by descriptions

	// Split into lines for easier processing
	lines := strings.Split(abilitiesSection, "\n")

	// Find where numbered images end
	numberedImagesEnd := 0
	for i, line := range lines {
		if abilityImageRe.MatchString(line) {
			numberedImagesEnd = i + 1
		}
	}

	// Look for ability names after numbered images
	// Ability names are typically ALL CAPS, 2+ words, on their own line
	abilityIndex := 0
	seenLargeImage := false

	for i := numberedImagesEnd; i < len(lines) && abilityIndex < 4; i++ {
		line := strings.TrimSpace(lines[i])

		// Skip empty lines and images
		if line == "" || strings.HasPrefix(line, "!") {
			if strings.HasPrefix(line, "!") {
				seenLargeImage = true
			}
			continue
		}

		// Check if this is an ability name (ALL CAPS or Title Case, reasonable length)
		isAllCaps := strings.ToUpper(line) == line && len(line) > 0
		isTitleCase := len(line) > 0 &&
			strings.ToUpper(line[:1]) == line[:1] &&
			(len(line) == 1 || strings.ToLower(line[1:]) == line[1:] ||
				strings.Count(line, " ") <= 3 && allWordsTitleCase(line))

		if seenLargeImage &&
			len(line) > 3 &&
			len(line) < 50 &&
			(isAllCaps || isTitleCase) &&
			strings.Count(line, " ") <= 3 {

			// Verify it's not a common footer word
			skipWords := []string{"ROLE", "SPECIAL", "ABILITIES", "BLOOD", "LANGUAGE", "VIOLENCE", "USERS", "INTERACT", "PURCHASES", "Download", "Twitter", "YouTube", "Instagram", "TikTok", "Facebook", "Discord", "Riot", "Games", "Privacy", "Notice", "Terms", "Service", "ESRB", "Check", "Session", "IFrame", "OIDC", "OP", "Auth", "Error"}
			isSkipWord := false
			lineUpper := strings.ToUpper(line)
			for _, word := range skipWords {
				if lineUpper == strings.ToUpper(word) {
					isSkipWord = true
					break
				}
			}
			if isSkipWord {
				continue
			}

			// This looks like an ability name
			abilityName := line

			// Collect description from following lines until next ability name or end
			descriptionLines := []string{}
			for j := i + 1; j < len(lines); j++ {
				nextLine := strings.TrimSpace(lines[j])

				// Stop if we hit another ability name (all caps or title case)
				if nextLine != "" &&
					!strings.HasPrefix(nextLine, "!") &&
					!strings.HasPrefix(nextLine, "-") &&
					!strings.HasPrefix(nextLine, "[") &&
					len(nextLine) > 3 && len(nextLine) < 50 {
					isNextAllCaps := strings.ToUpper(nextLine) == nextLine
					isNextTitleCase := len(nextLine) > 0 &&
						strings.ToUpper(nextLine[:1]) == nextLine[:1] &&
						(len(nextLine) == 1 || strings.ToLower(nextLine[1:]) == nextLine[1:] ||
							strings.Count(nextLine, " ") <= 3 && allWordsTitleCase(nextLine))
					if isNextAllCaps || isNextTitleCase {
						break
					}
				}

				// Stop if we hit footer patterns
				if strings.Contains(nextLine, "Download Riot") ||
					strings.Contains(nextLine, "Riot Games") ||
					strings.Contains(nextLine, "© 2020") {
					break
				}

				if nextLine != "" && !strings.HasPrefix(nextLine, "!") {
					descriptionLines = append(descriptionLines, nextLine)
				}
			}

			description := strings.Join(descriptionLines, " ")
			description = cleanDescription(description)

			// Get corresponding image and URL-encode it
			image := ""
			if abilityIndex+1 <= len(imageMap) {
				image = urlEncode(imageMap[abilityIndex+1])
			}

			abilities = append(abilities, Ability{
				Name:        abilityName,
				Description: description,
				Image:       image,
			})

			abilityIndex++
		}
	}

	return abilities
}

// urlEncode validates and normalizes a URL but keeps & characters intact
// The JSON encoder is configured to not escape HTML, so & will remain as &
func urlEncode(rawURL string) string {
	// Parse the URL to ensure it's valid and normalize it
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		// If parsing fails, return the original URL
		return rawURL
	}

	// Return the normalized URL with & characters intact
	// The JSON encoder with SetEscapeHTML(false) will preserve & as-is
	return parsedURL.String()
}

// allWordsTitleCase checks if all words in a string are title case (first letter uppercase)
func allWordsTitleCase(s string) bool {
	words := strings.Fields(s)
	for _, word := range words {
		if len(word) == 0 {
			continue
		}
		// Check if first letter is uppercase and rest is lowercase (or mixed for multi-word)
		first := word[0]
		if first < 'A' || first > 'Z' {
			return false
		}
		// Allow rest to be mixed case for compound words
	}
	return true
}

// cleanDescription removes markdown formatting and cleans up the description
func cleanDescription(desc string) string {
	// Remove markdown links but keep the text
	linkRe := regexp.MustCompile(`\[([^\]]+)\]\([^\)]+\)`)
	desc = linkRe.ReplaceAllString(desc, "$1")

	// Remove markdown images
	imageRe := regexp.MustCompile(`!\[.*?\]\(.*?\)`)
	desc = imageRe.ReplaceAllString(desc, "")

	// Remove extra whitespace
	desc = regexp.MustCompile(`\s+`).ReplaceAllString(desc, " ")
	desc = strings.TrimSpace(desc)

	// Remove common footer text patterns
	footerPatterns := []string{
		"Download Riot Mobile Companion App",
		"Twitter", "YouTube", "Instagram", "TikTok", "Facebook", "Discord",
		"Riot Games",
		"© 2020-2025 Riot Games",
		"Privacy Notice",
		"Terms of Service",
		"ESRB",
		"Blood", "Language", "Violence", "Users Interact", "In-Game Purchases",
		"Check Session IFrame",
		"OIDC OP Iframe",
		"Auth Error",
	}

	for _, pattern := range footerPatterns {
		if strings.Contains(desc, pattern) {
			// Split and take only the part before the footer
			idx := strings.Index(desc, pattern)
			if idx > 0 {
				desc = desc[:idx]
			}
		}
	}

	return strings.TrimSpace(desc)
}
