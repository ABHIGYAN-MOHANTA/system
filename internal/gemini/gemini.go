package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	apiURL     = "https://generativelanguage.googleapis.com/v1beta/models/gemini-3-flash-preview:generateContent"
	apiTimeout = 10 * time.Second
)

// getAPIKey returns the Gemini API key from environment variable
func getAPIKey() string {
	return os.Getenv("GEMINI_API_KEY")
}

// StatResponse represents the stat allocation from Gemini
type StatResponse struct {
	STR int `json:"str"`
	VIT int `json:"vit"`
	AGI int `json:"agi"`
	INT int `json:"int"`
}

// GeminiRequest is the request payload for Gemini API
type GeminiRequest struct {
	Contents []Content `json:"contents"`
}

type Content struct {
	Parts []Part `json:"parts"`
}

type Part struct {
	Text string `json:"text"`
}

// GeminiResponse is the response from Gemini API
type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

// GetLevelUpStats calls Gemini API to get stat allocation for a level-up
// habits is a list of habit names for context
// level is the new level the user has reached
// Returns the stat increases (not totals)
func GetLevelUpStats(habits []string, level int) (StatResponse, error) {
	pointsToAllocate := 4 // Points per level-up

	habitList := "None"
	if len(habits) > 0 {
		habitList = strings.Join(habits, ", ")
	}

	prompt := fmt.Sprintf(`You are the SYSTEM in a Solo Leveling-inspired habit tracker game. A hunter has just leveled up to level %d.

Their daily quests (habits) include: %s

Based on their progress and the nature of their quests, allocate stat points for this level-up. You have %d points to distribute across 4 stats: STR (Strength), VIT (Vitality), AGI (Agility), INT (Intelligence).

Consider:
- Physical/exercise habits like gym, running, workout → favor STR, VIT, AGI
- Learning/reading habits like study, read, learn → favor INT
- Meditation, sleep habits → favor VIT
- Speed/agility tasks → favor AGI
- General productivity → balanced distribution
- Be creative and thematic!

Respond with ONLY a valid JSON object, no markdown, no extra text:
{"str": X, "vit": Y, "agi": Z, "int": W}

Where X + Y + Z + W = %d. Each value must be 0 or greater.`, level, habitList, pointsToAllocate, pointsToAllocate)

	reqBody := GeminiRequest{
		Contents: []Content{
			{
				Parts: []Part{
					{Text: prompt},
				},
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return randomFallback(pointsToAllocate), fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), apiTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return randomFallback(pointsToAllocate), fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", getAPIKey())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return randomFallback(pointsToAllocate), fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return randomFallback(pointsToAllocate), fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return randomFallback(pointsToAllocate), fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var geminiResp GeminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return randomFallback(pointsToAllocate), fmt.Errorf("failed to parse response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return randomFallback(pointsToAllocate), fmt.Errorf("empty response from API")
	}

	responseText := geminiResp.Candidates[0].Content.Parts[0].Text
	responseText = strings.TrimSpace(responseText)

	// Extract JSON from response (handle markdown code blocks)
	jsonRegex := regexp.MustCompile(`\{[^}]+\}`)
	match := jsonRegex.FindString(responseText)
	if match == "" {
		return randomFallback(pointsToAllocate), fmt.Errorf("no JSON found in response: %s", responseText)
	}

	var stats StatResponse
	if err := json.Unmarshal([]byte(match), &stats); err != nil {
		return randomFallback(pointsToAllocate), fmt.Errorf("failed to parse stats JSON: %w", err)
	}

	// Validate the response
	total := stats.STR + stats.VIT + stats.AGI + stats.INT
	if total != pointsToAllocate {
		// Normalize to ensure correct total
		return normalizeStats(stats, pointsToAllocate), nil
	}

	return stats, nil
}

// randomFallback generates random stat allocation when API fails
func randomFallback(points int) StatResponse {
	rand.Seed(time.Now().UnixNano())
	stats := StatResponse{}
	remaining := points

	// Randomly allocate points
	stats.STR = rand.Intn(remaining + 1)
	remaining -= stats.STR

	if remaining > 0 {
		stats.VIT = rand.Intn(remaining + 1)
		remaining -= stats.VIT
	}

	if remaining > 0 {
		stats.AGI = rand.Intn(remaining + 1)
		remaining -= stats.AGI
	}

	stats.INT = remaining

	return stats
}

// normalizeStats adjusts stats to sum to the target points
func normalizeStats(stats StatResponse, targetPoints int) StatResponse {
	total := stats.STR + stats.VIT + stats.AGI + stats.INT
	if total == 0 {
		return randomFallback(targetPoints)
	}

	// Scale proportionally
	scale := float64(targetPoints) / float64(total)
	result := StatResponse{
		STR: int(float64(stats.STR) * scale),
		VIT: int(float64(stats.VIT) * scale),
		AGI: int(float64(stats.AGI) * scale),
		INT: int(float64(stats.INT) * scale),
	}

	// Adjust for rounding errors
	diff := targetPoints - (result.STR + result.VIT + result.AGI + result.INT)
	if diff > 0 {
		result.STR += diff
	} else if diff < 0 {
		if result.STR >= -diff {
			result.STR += diff
		} else if result.VIT >= -diff {
			result.VIT += diff
		} else if result.AGI >= -diff {
			result.AGI += diff
		} else {
			result.INT += diff
		}
	}

	return result
}
