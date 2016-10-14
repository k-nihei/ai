package main

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/line/line-bot-sdk-go/linebot"
	"github.com/sugyan/face-manager-linebot/inferences"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
	bot, err := linebot.New(
		os.Getenv("CHANNEL_SECRET"),
		os.Getenv("CHANNEL_TOKEN"),
	)
	if err != nil {
		log.Fatal(err)
	}
	app := &app{bot: bot}
	http.HandleFunc(os.Getenv("CALLBACK_PATH"), app.handler)
	http.HandleFunc("/thumbnail", thumbnailHandler)
	if err := http.ListenAndServe(":"+os.Getenv("PORT"), nil); err != nil {
		log.Fatal(err)
	}
}

type app struct {
	bot *linebot.Client
}

func (a *app) handler(w http.ResponseWriter, r *http.Request) {
	events, err := a.bot.ParseRequest(r)
	if err != nil {
		log.Printf("parse request error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	for _, event := range events {
		if event.Source.Type != linebot.EventSourceTypeUser {
			log.Printf("not from user: %v", event)
			continue
		}
		switch event.Type {
		case linebot.EventTypeMessage:
			if message, ok := event.Message.(*linebot.TextMessage); ok {
				log.Printf("text message from %s: %v", event.Source.UserID, message.Text)
				if err := a.sendCarousel(event.Source.UserID, event.ReplyToken); err != nil {
					log.Printf("send error: %v", err)
				}
			}
		case linebot.EventTypePostback:
			log.Printf("got postback: %s", event.Postback.Data)
			// <face-id>,<inference-id>
			ids := strings.Split(event.Postback.Data, ",")
			resultURL, err := inferences.Accept(event.Source.UserID, ids[1])
			if err != nil {
				log.Printf("accept error: %v", err)
				continue
			}
			if _, err := a.bot.ReplyMessage(
				event.ReplyToken,
				linebot.NewTemplateMessage(
					"template message",
					linebot.NewConfirmTemplate(
						fmt.Sprintf("id:%s を更新しました！", ids[0]),
						linebot.NewMessageTemplateAction("やっぱ違うわ", "やっぱ違うわ"),
						linebot.NewURITemplateAction("確認する", resultURL),
					),
				),
			).Do(); err != nil {
				log.Printf("send message error: %v", err)
				continue
			}
		default:
			log.Printf("not message/postback event: %v", event)
			continue
		}
	}
}

func (a *app) sendCarousel(userID, replyToken string) error {
	inferences, err := inferences.BulkFetch(userID)
	if err != nil {
		return err
	}
	if len(inferences) < 1 {
		return errors.New("empty inferences")
	}
	ids := rand.Perm(len(inferences))
	num := 5
	if len(ids) < num {
		num = len(ids)
	}
	re, err := regexp.Compile(` \((@\w+)\): `)
	if err != nil {
		return err
	}
	columns := make([]*linebot.CarouselColumn, 0, 5)
	for i := 0; i < num; i++ {
		inference := inferences[ids[i]]
		name := inference.Label.Name
		if inference.Label.Description != "" {
			name += " (" + strings.Replace(inference.Label.Description, "\r\n", ", ", -1) + ")"
		}
		thumbnailImageURL, err := url.Parse(os.Getenv("APP_URL") + "/thumbnail")
		if err != nil {
			return err
		}
		values := url.Values{}
		values.Set("image_url", inference.Face.ImageURL)
		submatch := re.FindStringSubmatch(inference.Face.Photo.Caption)
		if submatch != nil && len(submatch) > 1 {
			values.Set("from", submatch[1])
		}
		thumbnailImageURL.RawQuery = values.Encode()
		columns = append(
			columns,
			linebot.NewCarouselColumn(
				thumbnailImageURL.String(),
				fmt.Sprintf("id:%d [%.5f]", inference.Face.ID, inference.Score),
				name,
				linebot.NewURITemplateAction(
					"くわしく",
					inference.Face.Photo.SourceURL,
				),
				linebot.NewPostbackTemplateAction(
					"あってる",
					strings.Join(
						[]string{
							strconv.FormatUint(uint64(inference.Face.ID), 10),
							strconv.FormatUint(uint64(inference.ID), 10),
						},
						",",
					),
					"",
				),
			),
		)
	}
	if _, err = a.bot.ReplyMessage(
		replyToken,
		linebot.NewTemplateMessage("template message", linebot.NewCarouselTemplate(columns...)),
	).Do(); err != nil {
		return err
	}
	return nil
}
