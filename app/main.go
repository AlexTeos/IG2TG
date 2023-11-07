package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Davincible/goinsta/v3"
	"gopkg.in/ini.v1"
	tele "gopkg.in/telebot.v3"
)

type instaType struct {
	last_time int64
	cache     string
	insta     *goinsta.Instagram
}

func newInstaTypeCache(cache_file string) instaType {
	insta := instaType{last_time: 0, cache: cache_file}
	insta.insta, _ = goinsta.Import(insta.cache, true)
	insta.insta.OpenApp()
	defer insta.insta.Export(insta.cache)
	return insta
}

func newInstaTypeLogin(login string, password string) instaType {
	insta := instaType{last_time: 0, cache: "/ext/insta.cache"}
	insta.insta = goinsta.New(login, password)
	if err := insta.insta.Login(); err != nil {
		panic(err)
	}
	defer insta.insta.Export(insta.cache)
	return insta
}

func posts(insta *instaType) <-chan *goinsta.Item {
	сhannel := make(chan *goinsta.Item)

	go func() {
		for {
			items := insta.insta.Timeline.Items
			sort.Slice(items, func(i, j int) bool {
				return items[i].TakenAt < items[j].TakenAt
			})
			for i := 0; i < len(items); i++ {
				if insta.last_time < items[i].TakenAt {
					insta.last_time = items[i].TakenAt
					сhannel <- items[i]
				}
			}

			time.Sleep(5 * time.Second)
			if err := insta.insta.Timeline.Refresh(); err != nil {
				fmt.Printf("Refresh error: %s\n", err.Error())
			}
		}
		//close(сhannel)
	}()

	return сhannel
}

func isCommercial(item *goinsta.Item) bool {
	return item.CommercialityStatus != "not_commercial" || item.ProductType == "ad"
}

func main() {
	// load ini
	inidata, err := ini.Load("/ext/ig2tg.ini")
	if err != nil {
		os.Exit(1)
	}

	cache := "/ext/insta.cache"
	var insta instaType
	if _, err := os.Stat(cache); err == nil {
		insta = newInstaTypeCache(cache)
	} else {
		insta = newInstaTypeLogin(inidata.Section("instagram").Key("username").String(), inidata.Section("instagram").Key("password").String())
	}

	token := inidata.Section("telegram").Key("token").String()
	pref := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	b, err := tele.NewBot(pref)
	if err != nil {
		return
	}

	var user tele.User
	user.ID, _ = inidata.Section("telegram").Key("admin").Int64()

	for item := range posts(&insta) {
		if isCommercial(item) {
			continue
		}
		item.DownloadTo("/ext/media/0")

		var album tele.Album
		files, _ := filepath.Glob("/ext/media/0*")
		for i, file := range files {
			if strings.Contains(file, "jpg") || strings.Contains(file, "heic") {
				album = append(album, &tele.Photo{File: tele.FromDisk(file)})
			}
			if strings.Contains(file, "mp4") {
				album = append(album, &tele.Video{File: tele.FromDisk(file)})
			}

			if len(album) == 10 || (len(album) != 0 && i == len(files)-1) {
				_, err = b.SendAlbum(&user, album)
				if err != nil {
					fmt.Println(err)
				}
				album = nil
			}
		}

		if len(item.Caption.Text) != 0 {
			text := "TakenAt: " + time.Unix(item.TakenAt, 0).String()
			text += "\nCommercialityStatus: " + item.CommercialityStatus
			text += "\nProductType: " + item.ProductType
			text += "\nFollowing: " + strconv.FormatBool(item.User.Friendship.Following)
			text += "\nCaption: " + item.Caption.Text
			_, err = b.Send(&user, text)
			if err != nil {
				fmt.Println(err)
			}
		}

		files, _ = filepath.Glob("/ext/media/0*")
		for _, file := range files {
			os.Remove(file)
		}
	}
}
