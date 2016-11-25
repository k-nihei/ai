package app

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/line/line-bot-sdk-go/linebot"
	"github.com/sugyan/idol-face-linebot/recognizer"
)

func (app *BotApp) sendRecognized(messageID, replyToken string) error {
	// POST image data to recognition API
	res, err := app.linebot.GetMessageContent(messageID).Do()
	if err != nil {
		return err
	}
	defer res.Content.Close()
	bytes, err := ioutil.ReadAll(res.Content)
	if err != nil {
		return err
	}
	result, err := app.recognizerAdmin.RecognizeFaces(res.ContentType, bytes)
	if err != nil {
		return err
	}
	// check results, extract succeeded high scored results
	success := 0
	succeededFaces := make([]recognizer.RecognizedFace, 0, 5)
	sort.Sort(recognizer.ByTopValue(result.Faces))
	for _, face := range result.Faces {
		top := face.Recognize[0]
		if !(top.Label.ID > 0 && top.Value > 0.5) {
			continue
		}
		success++
		if len(succeededFaces) >= 5 {
			continue
		}
		succeededFaces = append(succeededFaces, face)
	}

	// encrypt message ID and pass URL
	key, err := app.encrypt(messageID)
	if err != nil {
		return err
	}
	var messages []linebot.Message
	if success > 0 {
		// success
		thumbnailImageURL, err := url.Parse(app.baseURL)
		if err != nil {
			return err
		}
		thumbnailImageURL.Path = path.Join(thumbnailImageURL.Path, "image")
		columns := columnsFromRecognizedFaces(succeededFaces, key, thumbnailImageURL.String())

		text := fmt.Sprintf("%d件の顔を識別しました\xf0\x9f\x98\x80", success)
		if len(result.Faces) > len(columns) {
			text = fmt.Sprintf("%d件中 %s", len(result.Faces), text)
		}
		altTextLines := []string{}
		for _, column := range columns {
			altTextLines = append(altTextLines, fmt.Sprintf("%s [%s]", column.Title, column.Text))
		}
		messages = []linebot.Message{
			linebot.NewTextMessage(text),
			linebot.NewTemplateMessage(
				strings.Join(altTextLines, "\n"),
				linebot.NewCarouselTemplate(columns...),
			),
		}
	} else {
		// failure
		var text string
		if len(result.Faces) > 0 {
			text = fmt.Sprintf("%d件の顔を検出しましたが、識別対象の人物ではなさそうです", len(result.Faces))
		} else {
			text = "顔を検出できませんでした"
		}
		messages = []linebot.Message{
			linebot.NewTextMessage(text + "\xf0\x9f\x98\x9e"),
		}
	}
	_, err = app.linebot.ReplyMessage(replyToken, messages...).Do()
	if err != nil {
		return err
	}
	return nil
}
