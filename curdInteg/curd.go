package curdInteg

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"github.com/gen2brain/beeep"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func GetTokenFromFile(filePath string) (string, error) {
	// Read the token from the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read token from file: %w", err)
	}

	// Convert the byte slice to a string and remove any surrounding whitespace or newlines
	token := strings.TrimSpace(string(data))

	return token, nil
}

func EditConfig(configFilePath string) {
	// Get the user's preferred editor from the EDITOR environment variable
	editor := os.Getenv("EDITOR")
	if editor == "" {
		// If EDITOR is not set, use system-specific defaults
		if runtime.GOOS == "windows" {
			// Try Notepad++ first
			if _, err := exec.LookPath("notepad++"); err == nil {
				editor = "notepad++"
			} else {
				editor = "notepad.exe"
			}
		} else {
			editor = "vim"
		}
	}

	// Construct the command to open the config file
	cmd := exec.Command(editor, configFilePath)

	// Set the command to run in the current terminal
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the editor command
	err := cmd.Run()
	if err != nil {
		CurdOut(fmt.Sprintf("Error opening config file: %v", err))
		return
	}

	CurdOut("Config file edited successfully.")
}

// ClearLogFile removes all contents from the specified log file
func ClearLogFile(logFile string) error {
	// Open the file with truncate flag to clear its contents
	file, err := os.OpenFile(logFile, os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	return nil
}

// LogData logs the input data into a specified log file with the format [LOG] time lineNumber: logData
func Log(data interface{}, logFile string) error {
	// Open or create the log file
	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer file.Close() // Ensure the file is closed when done

	// Attempt to marshal the data into JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// Get the caller information
	_, filename, lineNumber, ok := runtime.Caller(1) // Caller 1 gives the caller of LogData
	if !ok {
		return fmt.Errorf("unable to get caller information")
	}

	// Log the current time and the JSON representation along with caller info
	currentTime := time.Now().Format("2006/01/02 15:04:05")
	logMessage := fmt.Sprintf("[LOG] %s %s:%d: %s\n", currentTime, filename, lineNumber, jsonData)
	_, err = fmt.Fprint(file, logMessage) // Write to the file
	if err != nil {
		return err
	}

	return nil
}

// ClearScreen clears the terminal screen and saves the state
func ClearScreen() {
	fmt.Print("\033[?1049h") // Switch to alternate screen buffer
	fmt.Print("\033[2J")     // Clear the entire screen
	fmt.Print("\033[H")      // Move cursor to the top left
}

// RestoreScreen restores the original terminal state
func RestoreScreen() {
	fmt.Print("\033[?1049l") // Switch back to the main screen buffer
}

func ExitCurd(err error) {
	RestoreScreen()
	CurdOut("Have a great day!")
	if err != nil {
		CurdOut(err)
		if runtime.GOOS == "windows" {
			fmt.Println("Press Enter to exit")
			var wait string
			fmt.Scanln(&wait)
			os.Exit(1)
		} else {
			os.Exit(1)
		}
	}
	os.Exit(0)
}

func CurdOut(data interface{}) {
	userCurdConfig := GetGlobalConfig()
	if userCurdConfig == nil {
		userCurdConfig = &CurdConfig{}
	}
	if !userCurdConfig.RofiSelection {
		fmt.Println(fmt.Sprintf("%v", data))
	} else {
		switch runtime.GOOS {
		case "windows":
			err := beeep.Notify(
				"Curd",
				fmt.Sprintf("%v", data),
				"",
			)

			if err != nil {
				Log(fmt.Sprintf("Failed to send notification: %v", err), logFile)
			}
		case "linux":
			// Check if the input starts with "-i" for image notification
			dataStr := fmt.Sprintf("%v", data)
			if strings.HasPrefix(dataStr, "-i") && userCurdConfig.ImagePreview && userCurdConfig.RofiSelection {
				// Split the string to get image path and message
				parts := strings.SplitN(dataStr, " ", 3)
				if len(parts) == 3 {
					// Remove quotes from the message
					message := strings.Trim(parts[2], "\"")
					cmd := exec.Command("notify-send", "-i", parts[1], message)
					err := cmd.Run()
					if err != nil {
						Log(fmt.Sprintf("%v", cmd), logFile)
						Log(fmt.Sprintf("Failed to send notification: %v", err), logFile)
					}
				}
			} else {
				cmd := exec.Command("notify-send", dataStr)
				err := cmd.Run()
				if err != nil {
					Log(fmt.Sprintf("%v", cmd), logFile)
					Log(fmt.Sprintf("Failed to send notification: %v", err), logFile)
				}
			}
		}
	}
}

func UpdateAnimeEntry(userCurdConfig *CurdConfig, user *User, logFile string) {
	// Create update options map
	updateOptions := map[string]string{
		"CATEGORY": "Change Anime Category",
		"PROGRESS": "Change Progress",
		"SCORE":    "Add/Change Score",
	}

	// Select update option
	updateSelection, err := DynamicSelect(updateOptions, false)
	if err != nil {
		Log(fmt.Sprintf("Failed to select update option: %v", err), logFile)
		ExitCurd(fmt.Errorf("Failed to select update option"))
	}

	if updateSelection.Key == "-1" {
		ExitCurd(nil)
	}

	// Get user's anime list
	var animeListMap map[string]string
	var animeListMapPreview map[string]RofiSelectPreview

	if userCurdConfig.RofiSelection && userCurdConfig.ImagePreview {
		animeListMapPreview = make(map[string]RofiSelectPreview)
		// Include anime from all categories
		for _, entry := range user.AnimeList.Watching {
			title := entry.Media.Title.English
			if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
				title = entry.Media.Title.Romaji
			}
			animeListMapPreview[strconv.Itoa(entry.Media.ID)] = RofiSelectPreview{
				Title:      title,
				CoverImage: entry.CoverImage,
			}
		}
		for _, entry := range user.AnimeList.Completed {
			title := entry.Media.Title.English
			if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
				title = entry.Media.Title.Romaji
			}
			animeListMapPreview[strconv.Itoa(entry.Media.ID)] = RofiSelectPreview{
				Title:      title,
				CoverImage: entry.CoverImage,
			}
		}
		for _, entry := range user.AnimeList.Paused {
			title := entry.Media.Title.English
			if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
				title = entry.Media.Title.Romaji
			}
			animeListMapPreview[strconv.Itoa(entry.Media.ID)] = RofiSelectPreview{
				Title:      title,
				CoverImage: entry.CoverImage,
			}
		}
		for _, entry := range user.AnimeList.Dropped {
			title := entry.Media.Title.English
			if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
				title = entry.Media.Title.Romaji
			}
			animeListMapPreview[strconv.Itoa(entry.Media.ID)] = RofiSelectPreview{
				Title:      title,
				CoverImage: entry.CoverImage,
			}
		}
		for _, entry := range user.AnimeList.Planning {
			title := entry.Media.Title.English
			if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
				title = entry.Media.Title.Romaji
			}
			animeListMapPreview[strconv.Itoa(entry.Media.ID)] = RofiSelectPreview{
				Title:      title,
				CoverImage: entry.CoverImage,
			}
		}
	} else {
		animeListMap = make(map[string]string)
		// Include anime from all categories
		for _, entry := range user.AnimeList.Watching {
			title := entry.Media.Title.English
			if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
				title = entry.Media.Title.Romaji
			}
			animeListMap[strconv.Itoa(entry.Media.ID)] = title
		}
		for _, entry := range user.AnimeList.Completed {
			title := entry.Media.Title.English
			if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
				title = entry.Media.Title.Romaji
			}
			animeListMap[strconv.Itoa(entry.Media.ID)] = title
		}
		for _, entry := range user.AnimeList.Paused {
			title := entry.Media.Title.English
			if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
				title = entry.Media.Title.Romaji
			}
			animeListMap[strconv.Itoa(entry.Media.ID)] = title
		}
		for _, entry := range user.AnimeList.Dropped {
			title := entry.Media.Title.English
			if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
				title = entry.Media.Title.Romaji
			}
			animeListMap[strconv.Itoa(entry.Media.ID)] = title
		}
		for _, entry := range user.AnimeList.Planning {
			title := entry.Media.Title.English
			if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
				title = entry.Media.Title.Romaji
			}
			animeListMap[strconv.Itoa(entry.Media.ID)] = title
		}
	}

	// Select anime to update
	var selectedAnime SelectionOption
	if userCurdConfig.RofiSelection && userCurdConfig.ImagePreview {
		selectedAnime, err = DynamicSelectPreview(animeListMapPreview, false)
	} else {
		selectedAnime, err = DynamicSelect(animeListMap, false)
	}
	if err != nil {
		Log(fmt.Sprintf("Failed to select anime: %v", err), logFile)
		ExitCurd(fmt.Errorf("Failed to select anime"))
	}

	if selectedAnime.Key == "-1" {
		ExitCurd(nil)
	}

	animeID, err := strconv.Atoi(selectedAnime.Key)
	if err != nil {
		Log(fmt.Sprintf("Failed to convert anime ID: %v", err), logFile)
		ExitCurd(fmt.Errorf("Failed to convert anime ID"))
	}

	// After getting animeID, get the current anime entry
	selectedAnilistAnime, err := FindAnimeByAnilistID(user.AnimeList, selectedAnime.Key)
	if err != nil {
		Log(fmt.Sprintf("Can not find the anime in anilist animelist: %v", err), logFile)
		ExitCurd(fmt.Errorf("Can not find the anime in anilist animelist"))
	}
	ClearScreen()
	switch updateSelection.Key {
	case "CATEGORY":
		categories := map[string]string{
			"CURRENT":   "Currently Watching",
			"COMPLETED": "Completed",
			"PAUSED":    "On Hold",
			"DROPPED":   "Dropped",
			"PLANNING":  "Plan to Watch",
		}

		currentStatus := "None"
		if selectedAnilistAnime.Status != "" {
			currentStatus = categories[selectedAnilistAnime.Status]
		}
		CurdOut(fmt.Sprintf("Current category: %s", currentStatus))

		categorySelection, err := DynamicSelect(categories, false)
		if err != nil {
			Log(fmt.Sprintf("Failed to select category: %v", err), logFile)
			ExitCurd(fmt.Errorf("Failed to select category"))
		}

		if categorySelection.Key == "-1" {
			ExitCurd(nil)
		}

		err = UpdateAnimeStatus(user.Token, animeID, categorySelection.Key)
		if err != nil {
			Log(fmt.Sprintf("Failed to update anime status: %v", err), logFile)
			ExitCurd(fmt.Errorf("Failed to update anime status"))
		}

	case "PROGRESS":
		currentProgress := "None"
		if selectedAnilistAnime.Progress > 0 {
			currentProgress = strconv.Itoa(selectedAnilistAnime.Progress)
		}

		var progress string
		if userCurdConfig.RofiSelection {
			progress, err = GetUserInputFromRofi(fmt.Sprintf("Current progress: %s\nEnter new progress (episode number)", currentProgress))
			if err != nil {
				Log(fmt.Sprintf("Failed to get progress input: %v", err), logFile)
				ExitCurd(fmt.Errorf("Failed to get progress input"))
			}
		} else {
			CurdOut(fmt.Sprintf("Current progress: %s", currentProgress))
			CurdOut("Enter new progress (episode number):")
			fmt.Scanln(&progress)
		}

		progressNum, err := strconv.Atoi(progress)
		if err != nil {
			Log(fmt.Sprintf("Failed to convert progress to number: %v", err), logFile)
			ExitCurd(fmt.Errorf("Failed to convert progress to number"))
		}

		err = UpdateAnimeProgress(user.Token, animeID, progressNum)
		if err != nil {
			Log(fmt.Sprintf("Failed to update anime progress: %v", err), logFile)
			ExitCurd(fmt.Errorf("Failed to update anime progress"))
		}

	case "SCORE":
		currentScore := "None"
		if selectedAnilistAnime.Score > 0 {
			currentScore = strconv.Itoa(int(selectedAnilistAnime.Score))
		}
		CurdOut(fmt.Sprintf("Current score: %s", currentScore))

		err = RateAnime(user.Token, animeID)
		if err != nil {
			Log(fmt.Sprintf("Failed to update anime score: %v", err), logFile)
			ExitCurd(fmt.Errorf("Failed to update anime score"))
		}
	}

	CurdOut("Anime updated successfully!")
}

func UpdateCurd(repo, fileName string) error {
	// Get the path of the currently running executable
	executablePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("unable to find current executable: %v", err)
	}

	// Adjust file name for Windows
	if runtime.GOOS == "windows" {
		fileName += ".exe"
	}

	// GitHub release URL for curd
	url := fmt.Sprintf("https://github.com/%s/releases/latest/download/%s", repo, fileName)

	// Temporary path for the downloaded curd executable
	tmpPath := executablePath + ".tmp"

	// Download the curd executable
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download file: %v", err)
	}
	defer resp.Body.Close()

	// Check if the download was successful
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download file: received status code %d", resp.StatusCode)
	}

	// Create a new temporary file
	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %v", err)
	}
	defer out.Close()

	// Write the downloaded content to the temporary file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to save downloaded file: %v", err)
	}

	// Close and rename the temporary file to replace the current executable
	out.Close()

	// Replace the current executable with the downloaded curd
	if err := os.Rename(tmpPath, executablePath); err != nil {
		return fmt.Errorf("failed to replace the current executable: %v", err)
	}
	CurdOut(fmt.Sprintf("Downloaded curd executable to %v", executablePath))

	if runtime.GOOS != "windows" {
		// Ensure the new file has executable permissions
		if err := os.Chmod(executablePath, 0755); err != nil {
			return fmt.Errorf("failed to set permissions on the new file: %v", err)
		}
	}

	return nil
}

func AddNewAnime(userCurdConfig *CurdConfig, anime *Anime, user *User, databaseAnimes *[]Anime, logFile string) SelectionOption {
	var query string
	var animeMap map[string]string
	var animeMapPreview map[string]RofiSelectPreview
	var err error
	var anilistSelectedOption SelectionOption
	var anilistUserData map[string]interface{}
	var anilistUserDataPreview map[string]interface{}

	if userCurdConfig.RofiSelection {
		userInput, err := GetUserInputFromRofi("Enter the anime name")
		if err != nil {
			Log("Error getting user input: "+err.Error(), logFile)
			ExitCurd(fmt.Errorf("Error getting user input: " + err.Error()))
		}
		query = userInput
	} else {
		CurdOut("Enter the anime name:")
		fmt.Scanln(&query)
	}
	if userCurdConfig.RofiSelection && userCurdConfig.ImagePreview {
		animeMapPreview, err = SearchAnimeAnilistPreview(query, user.Token)
	} else {
		animeMap, err = SearchAnimeAnilist(query, user.Token)
	}
	if err != nil {
		Log(fmt.Sprintf("Failed to search anime: %v", err), logFile)
		ExitCurd(fmt.Errorf("Failed to search anime"))
	}
	if userCurdConfig.RofiSelection && userCurdConfig.ImagePreview {
		anilistSelectedOption, err = DynamicSelectPreview(animeMapPreview, false)
	} else {
		anilistSelectedOption, err = DynamicSelect(animeMap, false)
	}

	if anilistSelectedOption.Key == "-1" {
		ExitCurd(nil)
	}

	if err != nil {
		Log(fmt.Sprintf("No anime available: %v", err), logFile)
		ExitCurd(fmt.Errorf("No anime available"))
	}
	animeID, err := strconv.Atoi(anilistSelectedOption.Key)
	if err != nil {
		Log(fmt.Sprintf("Failed to convert anime ID to integer: %v", err), logFile)
		ExitCurd(fmt.Errorf("Failed to convert anime ID to integer"))
	}
	err = AddAnimeToWatchingList(animeID, user.Token)
	if err != nil {
		Log(fmt.Sprintf("Failed to add anime to watching list: %v", err), logFile)
		ExitCurd(fmt.Errorf("Failed to add anime to watching list"))
	}
	if user.Id == 0 {
		user.Id, user.Username, err = GetAnilistUserID(user.Token)
		if err != nil {
			Log(fmt.Sprintf("Failed to get user ID: %v", err), logFile)
			ExitCurd(fmt.Errorf("Failed to get user ID\nYou can reset the token by running `curd -change-token`"))
		}
	}
	if userCurdConfig.RofiSelection && userCurdConfig.ImagePreview {
		anilistUserDataPreview, err = GetUserDataPreview(user.Token, user.Id)
	} else {
		anilistUserData, err = GetUserData(user.Token, user.Id)
	}
	if err != nil {
		Log(fmt.Sprintf("Failed to get user data: %v", err), logFile)
		ExitCurd(fmt.Errorf("Failed to get user ID\nYou can reset the token by running `curd -change-token`"))
	}
	if userCurdConfig.RofiSelection && userCurdConfig.ImagePreview {
		user.AnimeList = ParseAnimeList(anilistUserDataPreview)
	} else {
		user.AnimeList = ParseAnimeList(anilistUserData)
	}

	return anilistSelectedOption
}

func SetupCurd(userCurdConfig *CurdConfig, anime *Anime, user *User, databaseAnimes *[]Anime, logFile string) {
	var err error
	var anilistUserData map[string]interface{}
	var anilistUserDataPreview map[string]interface{}

	// Filter anime list based on selected category
	var animeListMap map[string]string
	var animeListMapPreview map[string]RofiSelectPreview

	// Get user id, username and Anime list
	user.Id, user.Username, err = GetAnilistUserID(user.Token)
	if err != nil {
		Log(fmt.Sprintf("Failed to get user ID: %v", err), logFile)
		ExitCurd(fmt.Errorf("Failed to get user ID\nYou can reset the token by running `curd -change-token`"))
	}

	// Get the anime list data
	if userCurdConfig.RofiSelection && userCurdConfig.ImagePreview {
		anilistUserDataPreview, err = GetUserDataPreview(user.Token, user.Id)
		if err != nil {
			Log(fmt.Sprintf("Failed to get user data preview: %v", err), logFile)
			ExitCurd(fmt.Errorf("Failed to get user data preview"))
		}
		user.AnimeList = ParseAnimeList(anilistUserDataPreview)
	} else {
		anilistUserData, err = GetUserData(user.Token, user.Id)
		if err != nil {
			Log(fmt.Sprintf("Failed to get user data: %v", err), logFile)
			ExitCurd(fmt.Errorf("Failed to get user ID\nYou can reset the token by running `curd -change-token`"))
		}
		user.AnimeList = ParseAnimeList(anilistUserData)
	}

	// If continueLast flag is set, directly get the last watched anime
	if anime.Ep.ContinueLast {
		// Get the last anime ID from the curd_id file
		idFilePath := filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "curd_id")
		idBytes, err := os.ReadFile(idFilePath)
		if err != nil {
			Log("Error reading curd_id file: "+err.Error(), logFile)
			ExitCurd(fmt.Errorf("No last watched anime found"))
		}

		anilistID, err := strconv.Atoi(string(idBytes))
		if err != nil {
			Log("Error converting anilist ID: "+err.Error(), logFile)
			ExitCurd(fmt.Errorf("Invalid anime ID in curd_id file"))
		}

		// Find the anime in database
		animePointer := LocalFindAnime(*databaseAnimes, anilistID, "")
		if animePointer == nil {
			ExitCurd(fmt.Errorf("Last watched anime not found in database"))
		}

		// Set the anime details
		anime.AnilistId = animePointer.AnilistId
		// anime.AllanimeId = animePointer.AllanimeId
		// anime.Title = animePointer.Title
		// anime.Ep.Number = animePointer.Ep.Number
		// anime.Ep.Player.PlaybackTime = animePointer.Ep.Player.PlaybackTime
		// anime.Ep.Resume = true

	} else {
		// Skip category selection if Current flag is set
		var categorySelection SelectionOption
		if userCurdConfig.CurrentCategory {
			categorySelection = SelectionOption{
				Key:   "CURRENT",
				Label: "Currently Watching",
			}
		} else {
			// Create category selection map
			categories := map[string]string{
				"ALL":       "Show All",
				"CURRENT":   "Currently Watching",
				"PAUSED":    "On Hold",
				"PLANNING":  "Plan to Watch",
				"COMPLETED": "Completed",
				"DROPPED":   "Dropped",
				"UPDATE":    "Update Anime Entry",
				"UNTRACKED": "Untracked Watching",
			}

			// Select category using DynamicSelect
			var err error
			categorySelection, err = DynamicSelect(categories, false)
			if err != nil {
				Log(fmt.Sprintf("Failed to select category: %v", err), logFile)
				ExitCurd(fmt.Errorf("Failed to select category"))
			}

			if categorySelection.Key == "-1" {
				ExitCurd(nil)
			}

			// Handle UPDATE option
			if categorySelection.Key == "UPDATE" {
				ClearScreen()
				UpdateAnimeEntry(userCurdConfig, user, logFile)
				ExitCurd(nil)
			} else if categorySelection.Key == "UNTRACKED" {
				ClearScreen()
				WatchUntracked(userCurdConfig, logFile)
			}

			ClearScreen()
		}

		if userCurdConfig.RofiSelection && userCurdConfig.ImagePreview {
			animeListMapPreview = make(map[string]RofiSelectPreview)
			for _, entry := range getEntriesByCategory(user.AnimeList, categorySelection.Key) {
				title := entry.Media.Title.English
				if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
					title = entry.Media.Title.Romaji
				}
				animeListMapPreview[strconv.Itoa(entry.Media.ID)] = RofiSelectPreview{
					Title:      title,
					CoverImage: entry.CoverImage,
				}
			}
		} else {
			animeListMap = make(map[string]string)
			for _, entry := range getEntriesByCategory(user.AnimeList, categorySelection.Key) {
				title := entry.Media.Title.English
				if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
					title = entry.Media.Title.Romaji
				}
				animeListMap[strconv.Itoa(entry.Media.ID)] = title
			}
		}
	}

	var anilistSelectedOption SelectionOption
	var selectedAllanimeAnime SelectionOption
	var userQuery string

	if anime.Ep.ContinueLast {
		// Get the last watched anime ID from the curd_id file
		curdIDPath := filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "curd_id")
		curdIDBytes, err := os.ReadFile(curdIDPath)
		if err != nil {
			Log(fmt.Sprintf("Error reading curd_id file: %v", err), logFile)
			ExitCurd(fmt.Errorf("Error reading curd_id file"))
		}

		lastWatchedID, err := strconv.Atoi(strings.TrimSpace(string(curdIDBytes)))
		if err != nil {
			Log(fmt.Sprintf("Error converting curd_id to integer: %v", err), logFile)
			ExitCurd(fmt.Errorf("Error converting curd_id to integer"))
		}

		anime.AnilistId = lastWatchedID
		anilistSelectedOption.Key = strconv.Itoa(lastWatchedID)
	} else {
		// Select anime to watch (Anilist)
		var err error
		if userCurdConfig.RofiSelection && userCurdConfig.ImagePreview {
			anilistSelectedOption, err = DynamicSelectPreview(animeListMapPreview, true)
		} else {
			anilistSelectedOption, err = DynamicSelect(animeListMap, true)
		}
		if err != nil {
			Log(fmt.Sprintf("Error selecting anime: %v", err), logFile)
			ExitCurd(fmt.Errorf("Error selecting anime"))
		}

		Log(anilistSelectedOption, logFile)

		if anilistSelectedOption.Key == "-1" {
			ExitCurd(nil)
		}

		if anilistSelectedOption.Label == "add_new" || anilistSelectedOption.Key == "add_new" {
			anilistSelectedOption = AddNewAnime(userCurdConfig, anime, user, databaseAnimes, logFile)
		}

		anime.AnilistId, err = strconv.Atoi(anilistSelectedOption.Key)
		if err != nil {
			Log(fmt.Sprintf("Error converting Anilist ID: %v", err), logFile)
			ExitCurd(fmt.Errorf("Error converting Anilist ID"))
		}
	}
	// Find anime in Local history
	animePointer := LocalFindAnime(*databaseAnimes, anime.AnilistId, "")

	// Get anime entry
	selectedAnilistAnime, err := FindAnimeByAnilistID(user.AnimeList, anilistSelectedOption.Key)
	if err != nil {
		Log(fmt.Sprintf("Can not find the anime in anilist animelist: %v", err), logFile)
		ExitCurd(fmt.Errorf("Can not find the anime in anilist animelist"))
	}

	// Set anime entry
	anime.Title = selectedAnilistAnime.Media.Title
	anime.TotalEpisodes = selectedAnilistAnime.Media.Episodes
	anime.Ep.Number = selectedAnilistAnime.Progress + 1
	var animeList map[string]string
	userQuery = anime.Title.Romaji

	// if anime not found in database, find it in animeList
	if animePointer == nil {
		Log("Anime not found in database, searching in animeList...", logFile)
		// Get Anime list (All anime)
		Log(fmt.Sprintf("Searching for anime with query: %s, SubOrDub: %s", userQuery, userCurdConfig.SubOrDub), logFile)

		animeList, err = SearchAnime(string(userQuery), userCurdConfig.SubOrDub)
		Log(fmt.Sprintf("SearchAnime result - animeList: %+v, err: %v", animeList, err), logFile)
		if err != nil {
			Log(fmt.Sprintf("Failed to select anime: %v", err), logFile)
			ExitCurd(fmt.Errorf("Failed to select anime"))
		}
		if len(animeList) == 0 {
			ExitCurd(fmt.Errorf("No results found."))
		}

		// find anime in animeList
		anime.AllanimeId, err = FindKeyByValue(animeList, fmt.Sprintf("%v (%d episodes)", userQuery, selectedAnilistAnime.Media.Episodes))
		if err != nil {
			Log(fmt.Sprintf("Failed to find anime in animeList: %v", err), logFile)
		}

		// If unable to get Allanime id automatically get manually
		if anime.AllanimeId == "" {
			CurdOut("Failed to automatically select anime")
			selectedAllanimeAnime, err := DynamicSelect(animeList, false)

			if selectedAllanimeAnime.Key == "-1" {
				ExitCurd(nil)
			}

			if err != nil {
				// fmt.Println("No anime available")
				ExitCurd(fmt.Errorf("No anime available"))
			}
			anime.AllanimeId = selectedAllanimeAnime.Key
		}
	} else {
		// if anime found in database, use it
		anime.AllanimeId = animePointer.AllanimeId
		anime.Ep.Player.PlaybackTime = animePointer.Ep.Player.PlaybackTime
		if anime.Ep.Number == animePointer.Ep.Number {
			anime.Ep.Resume = true
		}
	}

	if selectedAllanimeAnime.Key == "-1" {
		ExitCurd(nil)
	}

	// If upstream is ahead, update the episode number
	if temp_anime, err := FindAnimeByAnilistID(user.AnimeList, strconv.Itoa(anime.AnilistId)); err == nil {
		if temp_anime.Progress > anime.Ep.Number {
			anime.Ep.Number = temp_anime.Progress
			anime.Ep.Player.PlaybackTime = 0
			anime.Ep.Resume = false
		}
	}

	if anime.TotalEpisodes == 0 {
		// Get updated anime data
		Log(selectedAllanimeAnime, logFile)
		updatedAnime, err := GetAnimeDataByID(anime.AnilistId, user.Token)
		Log(updatedAnime, logFile)
		if err != nil {
			Log(fmt.Sprintf("Error getting updated anime data: %v", err), logFile)
		} else {
			anime.TotalEpisodes = updatedAnime.TotalEpisodes
			Log(fmt.Sprintf("Updated total episodes: %d", anime.TotalEpisodes), logFile)
		}
	}

	if anime.TotalEpisodes == 0 { // If failed to get anime data
		CurdOut("Failed to get anime data. Attempting to retrieve from anime list.")
		animeList, err := SearchAnime(string(userQuery), userCurdConfig.SubOrDub)
		if err != nil {
			CurdOut(fmt.Sprintf("Failed to retrieve anime list: %v", err))
		} else {
			for allanimeId, label := range animeList {
				if allanimeId == anime.AllanimeId {
					// Extract total episodes from the label
					if matches := regexp.MustCompile(`\((\d+) episodes\)`).FindStringSubmatch(label); len(matches) > 1 {
						anime.TotalEpisodes, _ = strconv.Atoi(matches[1])
						CurdOut(fmt.Sprintf("Retrieved total episodes: %d", anime.TotalEpisodes))
						break
					}
				}
			}
		}

		if anime.TotalEpisodes == 0 {
			CurdOut("Still unable to determine total episodes.")
			CurdOut(fmt.Sprintf("Your AniList progress: %d", selectedAnilistAnime.Progress))
			var episodeNumber int
			if userCurdConfig.RofiSelection {
				userInput, err := GetUserInputFromRofi("Enter the episode you want to start from")
				if err != nil {
					Log("Error getting user input: "+err.Error(), logFile)
					ExitCurd(fmt.Errorf("Error getting user input: " + err.Error()))
				}
				episodeNumber, err = strconv.Atoi(userInput)
			} else {
				fmt.Print("Enter the episode you want to start from: ")
				fmt.Scanln(&episodeNumber)
			}
			anime.Ep.Number = episodeNumber
		} else {
			anime.Ep.Number = selectedAnilistAnime.Progress + 1
		}
	} else if anime.TotalEpisodes < anime.Ep.Number { // Handle weird cases
		Log(fmt.Sprintf("Weird case: anime.TotalEpisodes < anime.Ep.Number: %v < %v", anime.TotalEpisodes, anime.Ep.Number), logFile)
		var answer string
		if userCurdConfig.RofiSelection {
			userInput, err := GetUserInputFromRofi("Would like to start the anime from beginning? (y/n)")
			if err != nil {
				Log("Error getting user input: "+err.Error(), logFile)
				ExitCurd(fmt.Errorf("Error getting user input: " + err.Error()))
			}
			answer = userInput
		} else {
			fmt.Printf("Would like to start the anime from beginning? (y/n)\n")
			fmt.Scanln(&answer)
		}
		if answer == "y" {
			anime.Ep.Number = 1
		} else {
			anime.Ep.Number = anime.TotalEpisodes
		}
	}

}

// CreateOrWriteTokenFile creates the token file if it doesn't exist and writes the token to it
func WriteTokenToFile(token string, filePath string) error {
	// Extract the directory path
	dir := filepath.Dir(filePath)

	// Create all necessary parent directories
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directories: %v", err)
	}

	// Write the token to the file, creating it if it doesn't exist
	err := os.WriteFile(filePath, []byte(token), 0644)
	if err != nil {
		return fmt.Errorf("failed to write token to file: %v", err)
	}

	return nil
}

func StartCurd(userCurdConfig *CurdConfig, anime *Anime, logFile string) string {

	// Get episode link
	link, err := GetEpisodeURL(*userCurdConfig, anime.AllanimeId, anime.Ep.Number)
	if err != nil {
		// If unable to get episode link automatically get manually
		episodeList, err := EpisodesList(anime.AllanimeId, userCurdConfig.SubOrDub)
		if err != nil {
			CurdOut("No episode list found")
			RestoreScreen()
			os.Exit(1)
		}
		if userCurdConfig.RofiSelection {
			userInput, err := GetUserInputFromRofi(fmt.Sprintf("Enter the episode (%v episodes)", episodeList[len(episodeList)-1]))
			if err != nil {
				Log("Error getting user input: "+err.Error(), logFile)
				ExitCurd(fmt.Errorf("Error getting user input: " + err.Error()))
			}
			anime.Ep.Number, err = strconv.Atoi(userInput)
		} else {
			CurdOut(fmt.Sprintf("Enter the episode (%v episodes)", episodeList[len(episodeList)-1]))
			fmt.Scanln(&anime.Ep.Number)
		}
		link, err = GetEpisodeURL(*userCurdConfig, anime.AllanimeId, anime.Ep.Number)
		if err != nil {
			CurdOut("Failed to get episode link")
			os.Exit(1)
		}
		// anime.Ep.Links = link
	}
	anime.Ep.Links = link

	if len(anime.Ep.Links) == 0 {
		CurdOut("No episode links found")
		os.Exit(1)
	}

	Log(anime, logFile)

	// Write anime.AnilistId to curd_id in the storage path
	idFilePath := filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "curd_id")
	Log(fmt.Sprintf("idFilePath: %v", idFilePath), logFile)
	if err := os.MkdirAll(filepath.Dir(idFilePath), 0755); err != nil {
		Log(fmt.Sprintf("Failed to create directory for curd_id: %v", err), logFile)
	} else {
		if err := os.WriteFile(idFilePath, []byte(fmt.Sprintf("%d", anime.AnilistId)), 0644); err != nil {
			Log(fmt.Sprintf("Failed to write AnilistId to file: %v", err), logFile)
		}
	}

	// Display starting message with cover image and episode info
	if anime.CoverImage != "" && userCurdConfig.ImagePreview && userCurdConfig.RofiSelection {
		// Get the cached image path
		cacheDir := os.ExpandEnv("${HOME}/.cache/curd/images")
		filename := fmt.Sprintf("%x.jpg", md5.Sum([]byte(anime.CoverImage)))
		cachePath := filepath.Join(cacheDir, filename)

		// Display the image if it exists in cache
		_, err := os.Stat(cachePath)
		if err == nil {
			// File exists
			Log(fmt.Sprintf("Image found at %s", cachePath), logFile)
			CurdOut(fmt.Sprintf("-i %s \"%s - Episode %d\"", cachePath, GetAnimeName(*anime), anime.Ep.Number))
		} else {
			// File does not exist
			Log(fmt.Sprintf("Image does not exist at %s", cachePath), logFile)
			CurdOut(fmt.Sprintf("%s - Episode %d",
				GetAnimeName(*anime),
				anime.Ep.Number))

		}
	} else {
		CurdOut(fmt.Sprintf("%s - Episode %d", GetAnimeName(*anime), anime.Ep.Number))
	}
	mpvSocketPath, err := StartVideo(PrioritizeLink(anime.Ep.Links), []string{}, fmt.Sprintf("%s - Episode %d", GetAnimeName(*anime), anime.Ep.Number))

	if err != nil {
		Log("Failed to start mpv", logFile)
		os.Exit(1)
	}

	return mpvSocketPath
}

func CheckAndDownloadFiles(storagePath string, filesToCheck []string) error {
	// Create storage directory if it doesn't exist
	storagePath = os.ExpandEnv(storagePath)
	if err := os.MkdirAll(storagePath, 0755); err != nil {
		return fmt.Errorf("failed to create storage directory: %v", err)
	}

	// Base URL for downloading config files
	baseURL := "https://raw.githubusercontent.com/Wraient/curd/refs/heads/main/rofi/"

	// Check each file
	for _, fileName := range filesToCheck {
		filePath := filepath.Join(os.ExpandEnv(storagePath), fileName)

		// Skip if file already exists
		if _, err := os.Stat(filePath); err == nil {
			continue
		}

		// Download file if it doesn't exist
		resp, err := http.Get(baseURL + fileName)
		if err != nil {
			return fmt.Errorf("failed to download %s: %v", fileName, err)
		}
		defer resp.Body.Close()

		// Create the file
		out, err := os.Create(filePath)
		if err != nil {
			return fmt.Errorf("failed to create file %s: %v", fileName, err)
		}
		defer out.Close()

		// Write the content
		if _, err := io.Copy(out, resp.Body); err != nil {
			return fmt.Errorf("failed to write file %s: %v", fileName, err)
		}
	}

	return nil
}

func getEntriesByCategory(list AnimeList, category string) []Entry {
	switch category {
	case "ALL":
		// Combine all categories into one slice
		allEntries := make([]Entry, 0)
		allEntries = append(allEntries, list.Watching...)
		allEntries = append(allEntries, list.Completed...)
		allEntries = append(allEntries, list.Paused...)
		allEntries = append(allEntries, list.Dropped...)
		allEntries = append(allEntries, list.Planning...)
		return allEntries
	case "CURRENT":
		return list.Watching
	case "COMPLETED":
		return list.Completed
	case "PAUSED":
		return list.Paused
	case "DROPPED":
		return list.Dropped
	case "PLANNING":
		return list.Planning
	default:
		return []Entry{}
	}
}
