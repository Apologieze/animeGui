package main

import (
	curd "animeFyne/curdInteg"
	"fmt"
	"github.com/charmbracelet/log"
	"github.com/rl404/verniy"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

var localAnime []curd.Anime
var userCurdConfig curd.CurdConfig
var databaseFile string
var user curd.User

func startCurdInteg() {
	//discordClientId := "1287457464148820089"

	//var anime curd.Anime

	var homeDir string
	if runtime.GOOS == "windows" {
		homeDir = os.Getenv("USERPROFILE")
	} else {
		homeDir = os.Getenv("HOME")
	}

	configFilePath := filepath.Join(homeDir, ".config", "curd", "curd.conf")

	// load curd userCurdConfig
	var err error
	userCurdConfig, err = curd.LoadConfig(configFilePath)
	if err != nil {
		fmt.Println("Error loading config:", err)
		return
	}
	curd.SetGlobalConfig(&userCurdConfig)

	logFile := filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "debug.log")
	//curd.ClearLogFile(logFile)

	// Get the token from the token file
	user.Token, err = curd.GetTokenFromFile(filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "token"))
	if err != nil {
		log.Error("Error reading token", logFile)
	}
	if user.Token == "" {
		curd.ChangeToken(&userCurdConfig, &user)
	}

	if user.Id == 0 {
		user.Id, user.Username, err = curd.GetAnilistUserID(user.Token)
		if err != nil {
			log.Error(err)
		}
	}

	databaseFile = filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "curd_history.txt")
	localAnime = curd.LocalGetAllAnime(databaseFile)
	for _, anime := range localAnime {
		fmt.Println(anime)
	}
	/*fmt.Println(databaseFile)*/

	fmt.Println(user.Username)
	fmt.Println(user.Id)

	/*allId := localAnime[0].AllanimeId
	url, _ := curd.GetEpisodeURL(userCurdConfig, allId, 1)
	fmt.Println(curd.PrioritizeLink(url))*/
}

func SearchFromLocalAniId(id int) *curd.Anime {
	for _, anime := range localAnime {
		if anime.AnilistId == id {
			return &anime
		}
	}
	return nil
}

type AllAnimeIdData struct {
	Id   string
	Name string
}

func OnPlayButtonClick(animeName string, animeData *verniy.MediaList) {
	if animeData == nil {
		log.Error("Anime data is nil")
		return
	}
	var allAnimeId string
	animeProgress := 0
	if animeData.Progress != nil && animeData.Media.Episodes != nil {
		animeProgress = min(*animeData.Progress, *animeData.Media.Episodes-1)
	}
	animePointer := SearchFromLocalAniId(animeData.Media.ID)
	if animePointer == nil {
		allAnimeId = searchAllAnimeData(animeName, animeData.Media.Episodes, animeProgress)
		if allAnimeId == "" {
			log.Error("Failed to get allAnimeId")
			return
		}
		err := curd.LocalUpdateAnime(databaseFile, animeData.Media.ID, allAnimeId, animeProgress, 0, 0, animeName)
		if err != nil {
			log.Error("Can't update database file", err)
			return
		} else {
			log.Info("Successfully updated database file")
		}
	} else {
		fmt.Println(*animePointer)
		allAnimeId = animePointer.AllanimeId
	}
	log.Info("AllAnimeId!!!!!:", allAnimeId)

	animeProgress++

	log.Info("Anime Progress:", animeProgress)

	url, err := curd.GetEpisodeURL(userCurdConfig, allAnimeId, animeProgress)
	if err != nil {
		log.Error(err)
		return
	}
	finalLink := curd.PrioritizeLink(url)
	fmt.Println(finalLink)

	mpvSocketPath, err := curd.StartVideo(finalLink, []string{}, fmt.Sprintf("%s - Episode %d", animeName, animeProgress))
	if err != nil {
		log.Error(err)
		return
	}
	fmt.Println("MPV Socket Path:", mpvSocketPath)
	playingAnime := curd.Anime{AnilistId: animeData.Media.ID, AllanimeId: allAnimeId}
	playingAnime.Ep.Player.SocketPath = mpvSocketPath
	playingAnime.Title.English = animeName
	playingAnime.Ep.Number = animeProgress - 1
	playingAnime.TotalEpisodes = *animeData.Media.Episodes
	if animePointer != nil {
		fmt.Println("AnimePointer:", animePointer.Ep.Number, playingAnime.Ep.Number)
		if animePointer.Ep.Number == playingAnime.Ep.Number {
			playingAnime.Ep.Player.PlaybackTime = animePointer.Ep.Player.PlaybackTime
		}
	}
	playingAnimeLoop(playingAnime, animeData)
}

func searchAllAnimeData(animeName string, epNumber *int, animeProgress int) string {
	searchAnimeResult, err := curd.SearchAnime(animeName, "sub")
	if err != nil {
		log.Error(err)
	}

	var AllanimeId string

	if epNumber != nil {
		AllanimeId, err = curd.FindKeyByValue(searchAnimeResult, fmt.Sprintf("%v (%d episodes)", animeName, *epNumber))
		if err != nil {
			log.Error("Failed to find anime in animeList:", err)
		}

	}

	// If unable to get Allanime id automatically get manually
	if AllanimeId == "" {
		var keyValueArray []AllAnimeIdData
		log.Error("Failed to link anime automatically")
		for key, value := range searchAnimeResult {
			keyValueArray = append(keyValueArray, AllAnimeIdData{Id: key, Name: value})
		}

		selectCorrectLinking(keyValueArray, animeName, animeProgress)
		return ""
	}
	fmt.Println(AllanimeId)
	return AllanimeId
}

func playingAnimeLoop(playingAnime curd.Anime, animeData *verniy.MediaList) {
	fmt.Println(playingAnime.Ep.Player.PlaybackTime, "ah oue")
	// Get video duration
	go func() {
		for {
			time.Sleep(1 * time.Second)
			if playingAnime.Ep.Duration == 0 {
				// Get video duration
				durationPos, err := curd.MPVSendCommand(playingAnime.Ep.Player.SocketPath, []interface{}{"get_property", "duration"})
				if err != nil {
					log.Error("Error getting video duration: " + err.Error())
				} else if durationPos != nil {
					if duration, ok := durationPos.(float64); ok {
						playingAnime.Ep.Duration = int(duration + 0.5) // Round to nearest integer
						log.Infof("Video duration: %d seconds", playingAnime.Ep.Duration)

						if playingAnime.Ep.Player.PlaybackTime > 10 {
							_, err := curd.SeekMPV(playingAnime.Ep.Player.SocketPath, max(0, playingAnime.Ep.Player.PlaybackTime-5))
							if err != nil {
								log.Error("Error seeking video: " + err.Error())
							}
						} else {
							log.Error("Error seeking video: playback time is", playingAnime.Ep.Player.PlaybackTime)
						}
						break
					} else {
						log.Error("Error: duration is not a float64")
					}
				}
			}
		}
		log.Info("YIPIIIIIII", playingAnime.Ep.Duration)

		for {
			time.Sleep(1 * time.Second)
			timePos, err := curd.MPVSendCommand(playingAnime.Ep.Player.SocketPath, []interface{}{"get_property", "time-pos"})
			if err != nil {
				log.Error("Error getting video position: " + err.Error())
				fmt.Println("EH en vrai", playingAnime.Ep.Player.PlaybackTime, playingAnime.Ep.Duration)
				percentageWatched := curd.PercentageWatched(playingAnime.Ep.Player.PlaybackTime, playingAnime.Ep.Duration)

				if int(percentageWatched) >= userCurdConfig.PercentageToMarkComplete {
					playingAnime.Ep.Number++
					playingAnime.Ep.Player.PlaybackTime = 0
					var newProgress int = playingAnime.Ep.Number
					animeData.Progress = &newProgress
					go UpdateAnimeProgress(playingAnime.AnilistId, playingAnime.Ep.Number)
					episodeNumber.SetText(fmt.Sprintf("Episode %d/%d", playingAnime.Ep.Number, playingAnime.TotalEpisodes))
				}

				err := curd.LocalUpdateAnime(databaseFile, playingAnime.AnilistId, playingAnime.AllanimeId, playingAnime.Ep.Number, playingAnime.Ep.Player.PlaybackTime, 0, playingAnime.Title.English)
				if err == nil {
					log.Info("Successfully updated database file")
				}
				localAnime = curd.LocalGetAllAnime(databaseFile)
				break
			}
			if timePos != nil && playingAnime.Ep.Duration != 0 {
				if timing, ok := timePos.(float64); ok {
					playingAnime.Ep.Player.PlaybackTime = int(timing + 0.5)
					log.Infof("Video position: %d seconds", playingAnime.Ep.Player.PlaybackTime)
				} else {
					log.Error("Error: time-pos is not a float64")
				}

			}

		}
	}()

}

func UpdateAnimeProgress(animeId int, episode int) {
	err := curd.UpdateAnimeProgress(user.Token, animeId, episode)
	if err != nil {
		log.Error(err)
	}

}
