package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	EXPPerQuest      = 10
	EXPPerLevel      = 100
	DataDir          = "data"
	DefaultLevel     = 1
	DefaultResetHour = 4 // 4 AM
)

type Habit struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type UserData struct {
	Username         string                     `json:"username"`
	PasswordHash     string                     `json:"password_hash"`
	Habits           []Habit                    `json:"habits"`
	Level            int                        `json:"level"`
	EXP              int                        `json:"exp"`
	STR              int                        `json:"str"`               // Strength
	VIT              int                        `json:"vit"`               // Vitality
	AGI              int                        `json:"agi"`               // Agility
	INT              int                        `json:"int"`               // Intelligence
	CurrentStreak    int                        `json:"current_streak"`    // Days in a row completing all quests
	LongestStreak    int                        `json:"longest_streak"`    // Personal best streak
	LastCompleteDay  string                     `json:"last_complete_day"` // Last day all quests completed
	DailyCompletions map[string]map[string]bool `json:"daily_completions"`
	DayResetHour     int                        `json:"day_reset_hour"` // Hour (0-23) when daily quests reset
	mu               sync.Mutex                 `json:"-"`
}

func (u *UserData) TodayKey() string {
	now := time.Now()
	// If current time is before reset hour, use previous calendar day
	if now.Hour() < u.DayResetHour {
		now = now.Add(-24 * time.Hour)
	}
	return now.Format("2006-01-02")
}

func (u *UserData) CompletedToday(habitID string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.DailyCompletions == nil {
		return false
	}
	today := u.TodayKey()
	day, ok := u.DailyCompletions[today]
	if !ok {
		return false
	}
	return day[habitID]
}

func (u *UserData) ToggleToday(habitID string) (gainedEXP bool, leveledUp bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	today := u.TodayKey()
	if u.DailyCompletions == nil {
		u.DailyCompletions = make(map[string]map[string]bool)
	}
	if u.DailyCompletions[today] == nil {
		u.DailyCompletions[today] = make(map[string]bool)
	}
	was := u.DailyCompletions[today][habitID]
	u.DailyCompletions[today][habitID] = !was
	gainedEXP = !was // only gain EXP when marking complete
	if gainedEXP {
		u.EXP += EXPPerQuest
		for u.EXP >= u.Level*EXPPerLevel {
			u.Level++
			leveledUp = true
		}
	} else {
		u.EXP -= EXPPerQuest
		if u.EXP < 0 {
			u.EXP = 0
		}
		for u.Level > 1 && u.EXP < (u.Level-1)*EXPPerLevel {
			u.Level--
		}
	}
	return gainedEXP, leveledUp
}

// AllQuestsCompletedToday checks if all habits are completed for today
func (u *UserData) AllQuestsCompletedToday() bool {
	if len(u.Habits) == 0 {
		return false
	}
	today := u.TodayKey()
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.DailyCompletions == nil || u.DailyCompletions[today] == nil {
		return false
	}
	for _, h := range u.Habits {
		if !u.DailyCompletions[today][h.ID] {
			return false
		}
	}
	return true
}

// UpdateStreak updates the streak based on completion status
func (u *UserData) UpdateStreak() {
	today := u.TodayKey()
	u.mu.Lock()
	defer u.mu.Unlock()

	// Check if all quests completed today
	allComplete := true
	if len(u.Habits) == 0 {
		allComplete = false
	} else if u.DailyCompletions == nil || u.DailyCompletions[today] == nil {
		allComplete = false
	} else {
		for _, h := range u.Habits {
			if !u.DailyCompletions[today][h.ID] {
				allComplete = false
				break
			}
		}
	}

	if !allComplete {
		// If today was complete but now isn't (unchecked a quest)
		if u.LastCompleteDay == today {
			u.LastCompleteDay = ""
			u.CurrentStreak--
			if u.CurrentStreak < 0 {
				u.CurrentStreak = 0
			}
		}
		return
	}

	// All quests completed today
	if u.LastCompleteDay == today {
		// Already counted today
		return
	}

	// Check if yesterday was the last complete day (streak continues)
	yesterday := time.Now()
	if yesterday.Hour() < u.DayResetHour {
		yesterday = yesterday.Add(-24 * time.Hour)
	}
	yesterday = yesterday.Add(-24 * time.Hour)
	yesterdayKey := yesterday.Format("2006-01-02")

	if u.LastCompleteDay == yesterdayKey {
		// Streak continues
		u.CurrentStreak++
	} else if u.LastCompleteDay == "" {
		// First completion or streak was broken
		u.CurrentStreak = 1
	} else {
		// Streak broken, start fresh
		u.CurrentStreak = 1
	}

	u.LastCompleteDay = today
	if u.CurrentStreak > u.LongestStreak {
		u.LongestStreak = u.CurrentStreak
	}
}

func (u *UserData) EXPForNextLevel() int {
	return u.Level * EXPPerLevel
}

func (u *UserData) EXPInCurrentLevel() int {
	base := (u.Level - 1) * EXPPerLevel
	return u.EXP - base
}

// NextResetTime returns the exact time of the next day reset
func (u *UserData) NextResetTime() time.Time {
	now := time.Now()
	// Create today's reset time
	todayReset := time.Date(now.Year(), now.Month(), now.Day(), u.DayResetHour, 0, 0, 0, now.Location())
	// If we've already passed today's reset, use tomorrow's
	if now.After(todayReset) || now.Equal(todayReset) {
		return todayReset.Add(24 * time.Hour)
	}
	return todayReset
}

// TimeUntilReset returns the duration until the next day reset
func (u *UserData) TimeUntilReset() time.Duration {
	return time.Until(u.NextResetTime())
}

// UpdateDayResetHour updates the reset hour with validation
func (u *UserData) UpdateDayResetHour(hour int) error {
	if hour < 0 || hour > 23 {
		return fmt.Errorf("reset hour must be between 0 and 23")
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	u.DayResetHour = hour
	return nil
}

func (u *UserData) AddHabit(name string) Habit {
	u.mu.Lock()
	defer u.mu.Unlock()
	id := fmt.Sprintf("h_%d", time.Now().UnixNano())
	h := Habit{ID: id, Name: name}
	u.Habits = append(u.Habits, h)
	return h
}

func (u *UserData) RemoveHabit(index int) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if index < 0 || index >= len(u.Habits) {
		return false
	}
	u.Habits = append(u.Habits[:index], u.Habits[index+1:]...)
	return true
}

func (u *UserData) HabitByIndex(i int) (Habit, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if i < 0 || i >= len(u.Habits) {
		return Habit{}, false
	}
	return u.Habits[i], true
}

// ApplyLevelUpStats adds the given stat increases to the user's stats
func (u *UserData) ApplyLevelUpStats(str, vit, agi, intel int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.STR += str
	u.VIT += vit
	u.AGI += agi
	u.INT += intel
}

// GetHabitNames returns a list of all habit names
func (u *UserData) GetHabitNames() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	names := make([]string, len(u.Habits))
	for i, h := range u.Habits {
		names[i] = h.Name
	}
	return names
}

func userPath(username string) string {
	safe := filepath.Clean(username)
	if safe == "" || safe == "." || safe == ".." {
		safe = "default"
	}
	return filepath.Join(DataDir, safe+".json")
}

func LoadUser(username string) (*UserData, error) {
	path := userPath(username)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var u UserData
	if err := json.Unmarshal(data, &u); err != nil {
		return nil, err
	}
	if u.DailyCompletions == nil {
		u.DailyCompletions = make(map[string]map[string]bool)
	}
	if u.Level < 1 {
		u.Level = DefaultLevel
	}
	if u.DayResetHour < 0 || u.DayResetHour > 23 {
		u.DayResetHour = DefaultResetHour
	}
	// Initialize stats with base values for backwards compatibility
	const baseStats = 10
	if u.STR == 0 {
		u.STR = baseStats + u.Level
	}
	if u.VIT == 0 {
		u.VIT = baseStats + u.Level
	}
	if u.AGI == 0 {
		u.AGI = baseStats + u.Level
	}
	if u.INT == 0 {
		u.INT = baseStats + u.Level
	}
	return &u, nil
}

func UserExists(username string) bool {
	path := userPath(username)
	_, err := os.Stat(path)
	return err == nil
}

func AuthUser(username, password string) (*UserData, error) {
	username = strings.TrimSpace(strings.ToLower(username))
	if username == "" {
		return nil, fmt.Errorf("username required")
	}
	u, err := LoadUser(username)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("unknown user")
		}
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid password")
	}
	return u, nil
}

func CreateUser(username, password string) (*UserData, error) {
	username = strings.TrimSpace(strings.ToLower(username))
	if username == "" {
		return nil, fmt.Errorf("username required")
	}
	if len(password) < 4 {
		return nil, fmt.Errorf("password must be at least 4 characters")
	}
	if UserExists(username) {
		return nil, fmt.Errorf("username already taken")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	const baseStats = 10
	u := &UserData{
		Username:         username,
		PasswordHash:     string(hash),
		Habits:           []Habit{},
		Level:            DefaultLevel,
		EXP:              0,
		STR:              baseStats + DefaultLevel,
		VIT:              baseStats + DefaultLevel,
		AGI:              baseStats + DefaultLevel,
		INT:              baseStats + DefaultLevel,
		DailyCompletions: make(map[string]map[string]bool),
		DayResetHour:     DefaultResetHour,
	}
	if err := SaveUser(u); err != nil {
		return nil, err
	}
	return u, nil
}

func SaveUser(u *UserData) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	path := userPath(u.Username)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(u, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
