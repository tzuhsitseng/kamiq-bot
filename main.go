package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/line/line-bot-sdk-go/v7/linebot"
	"github.com/tzuhsitseng/kamiq-bot/repositories"
)

type CatcherStatus int

type CatcherInfo struct {
	LicensePlateNumber string
	UserID             string
	UserName           string
	HauntedPlaces      string
	SelfIntro          string
	CoverURL           string
	GroupIDs           []string
	GroupNames         []string
}

type Info struct {
	Keyword  string
	Question string
	Answer   string
	BtnText  string
	URL      string
	ImageURL string
}

type ImgurResp struct {
	Data struct {
		Link string `json:"link"`
	} `json:"data"`
}

const (
	CatcherStatusLicensePlateNumber CatcherStatus = iota + 1
	CatcherStatusHauntedPlaces
	CatcherStatusSelfIntro
	CatcherStatusCoverURL
)

var bot *linebot.Client
var catcherRepo repositories.CatchersRepository
var imgurClientID string

var (
	regionalGroupIDs = map[string]string{
		//"C9e940992c239eb57663525cde6b26a6b": "bot 測試群",
		//"Ca23770eb185ea43e725a71cda54a7e9e": "退休生活",
		"Cb6cfd28af50d41e8dd69b83efa7a5d26": "北一群",
		"Cc36a07572245c408431d11bd7fd94a45": "北二群",
		"C70b22d41c71fbccd1f557f6010f1d3e5": "中區群",
		"Cff9579c1947754d35387850add5c437e": "南區群",
	}

	allGroupIDs = map[string]string{
		"C193b9f94b6774670be047cf22575d99f": "大一群",
		"C1ee14832848258d925ab801cb91fd76e": "大二群",
		"C9fff1abaab5eddda37095a31b11b9335": "大三群",
		"Cb6cfd28af50d41e8dd69b83efa7a5d26": "北一群",
		"Cc36a07572245c408431d11bd7fd94a45": "北二群",
		"C70b22d41c71fbccd1f557f6010f1d3e5": "中區群",
		"Cff9579c1947754d35387850add5c437e": "南區群",
	}
)

var (
	catchers        = sync.Map{}
	catcherStatuses = sync.Map{}
)

var (
	oldLicensePlateNumberRegexp = regexp.MustCompile("^[0-9]{4}\\-[A-Za-z0-9]{2}$")
	newLicensePlateNumberRegexp = regexp.MustCompile("^[A-Za-z]{3}\\-[0-9]{4}$")
)

func main() {
	var err error
	bot, err = linebot.New(os.Getenv("CHANNEL_SECRET"), os.Getenv("CHANNEL_ACCESS_TOKEN"))
	if err != nil {
		panic("cannot create bot client")
	}
	http.HandleFunc("/callback", callbackHandler)
	imgurClientID = os.Getenv("IMGUR_CLIENT_ID")
	catcherRepo = repositories.NewCatcherRepository()
	http.ListenAndServe(fmt.Sprintf(":%s", os.Getenv("PORT")), nil)
}

func callbackHandler(w http.ResponseWriter, r *http.Request) {
	events, err := bot.ParseRequest(r)

	if err != nil {
		if err == linebot.ErrInvalidSignature {
			w.WriteHeader(400)
		} else {
			w.WriteHeader(500)
		}
		return
	}

	for _, event := range events {
		sourceType := event.Source.Type

		if sourceType == linebot.EventSourceTypeUser {
			userID := event.Source.UserID
			log.Printf("user id: %s", userID)

			if event.Type == linebot.EventTypeMessage {
				switch message := event.Message.(type) {
				case *linebot.TextMessage:
					if message.Text == "一起抓抓樂" {
						authorized := false
						for gid := range regionalGroupIDs {
							if _, err := bot.GetGroupMemberProfile(gid, userID).Do(); err == nil {
								authorized = true
								break
							}
						}
						if !authorized {
							if _, err := bot.ReplyMessage(event.ReplyToken, linebot.NewTextMessage("授權未通過，請確認已在 KamiQ 車主限定群")).Do(); err != nil {
								log.Println(err)
							}
							return
						}

						catchers.Store(userID, CatcherInfo{UserID: userID})
						catcherStatuses.Store(userID, CatcherStatusLicensePlateNumber)
						if _, err := bot.ReplyMessage(event.ReplyToken, linebot.NewTextMessage("授權通過，請輸入車牌號碼含-，例如: ABC-1234")).Do(); err != nil {
							log.Println(err)
							return
						}
						return
					}

					catcherStatus, ok := catcherStatuses.Load(userID)
					if ok {
						switch catcherStatus {
						case CatcherStatusLicensePlateNumber:
							if !newLicensePlateNumberRegexp.MatchString(message.Text) && !oldLicensePlateNumberRegexp.MatchString(message.Text) {
								if _, err := bot.ReplyMessage(event.ReplyToken, linebot.NewTextMessage("錯誤的車牌號碼格式，請重新輸入")).Do(); err != nil {
									log.Println(err)
									return
								}
								return
							}
							if catcher, ok := catchers.Load(userID); ok {
								if catcherInfo, ok := catcher.(CatcherInfo); ok {
									catcherInfo.LicensePlateNumber = strings.ToUpper(message.Text)
									catchers.Store(userID, catcherInfo)
									catcherStatuses.Store(userID, CatcherStatusHauntedPlaces)
									if _, err := bot.ReplyMessage(event.ReplyToken, linebot.NewTextMessage("設定完成，請輸入日常工作生活區域，例如: 龜山島")).Do(); err != nil {
										log.Println(err)
									}
								}
							}
							return

						case CatcherStatusHauntedPlaces:
							if catcher, ok := catchers.Load(userID); ok {
								if catcherInfo, ok := catcher.(CatcherInfo); ok {
									catcherInfo.HauntedPlaces = message.Text
									catchers.Store(userID, catcherInfo)
									catcherStatuses.Store(userID, CatcherStatusSelfIntro)
									if _, err := bot.ReplyMessage(event.ReplyToken, linebot.NewTextMessage("設定完成\n請輸入自我介紹 (限 50 字)\n若無自介請輸入 52~~\n自介將會顯示我愛蛇哥")).Do(); err != nil {
										log.Println(err)
									}
								}
							}
							return

						case CatcherStatusSelfIntro:
							if utf8.RuneCountInString(message.Text) > 50 {
								if _, err := bot.ReplyMessage(event.ReplyToken, linebot.NewTextMessage("已超出字數上限 (50)，請重新輸入")).Do(); err != nil {
									log.Println(err)
									return
								}
								return
							}
							if message.Text == "52~~" {
								message.Text = "我愛蛇哥"
							}
							if catcher, ok := catchers.Load(userID); ok {
								if catcherInfo, ok := catcher.(CatcherInfo); ok {
									catcherInfo.SelfIntro = message.Text
									catchers.Store(userID, catcherInfo)
									catcherStatuses.Store(userID, CatcherStatusCoverURL)
									if _, err := bot.ReplyMessage(event.ReplyToken, linebot.NewTextMessage("設定完成，請上傳最得意的愛車照片\n建議橫式照片，較不易被裁切")).Do(); err != nil {
										log.Println(err)
									}
								}
							}
							return
						}
					}
				case *linebot.ImageMessage:
					catcherStatus, ok := catcherStatuses.Load(userID)
					if !ok || catcherStatus != CatcherStatusCoverURL {
						return
					}

					resp, err := bot.GetMessageContent(message.ID).Do()
					if err != nil {
						log.Println(err)
						return
					}

					coverURL := uploadImgur(resp.Content)
					if coverURL == "" {
						return
					}
					log.Println(fmt.Sprintf("image url: %s", coverURL))

					ownGroupIDs := make([]string, 0)
					ownGroupNames := make([]string, 0)
					userName := ""
					for groupID, groupName := range regionalGroupIDs {
						if profile, err := bot.GetGroupMemberProfile(groupID, userID).Do(); err == nil {
							ownGroupIDs = append(ownGroupIDs, groupID)
							ownGroupNames = append(ownGroupNames, groupName)
							userName = profile.DisplayName
						}
					}
					if len(ownGroupIDs) == 0 {
						return
					}

					if catcher, ok := catchers.Load(userID); ok {
						if catcherInfo, ok := catcher.(CatcherInfo); ok {
							catcherInfo.CoverURL = coverURL
							catcherInfo.GroupIDs = ownGroupIDs
							catcherInfo.GroupNames = ownGroupNames
							catcherInfo.UserName = userName
							finalCatchers := make([]repositories.Catcher, 0, len(ownGroupIDs))
							for idx, groupID := range ownGroupIDs {
								finalCatchers = append(finalCatchers, repositories.Catcher{
									LicensePlateNumber: catcherInfo.LicensePlateNumber,
									UserID:             catcherInfo.UserID,
									UserName:           catcherInfo.UserName,
									HauntedPlaces:      catcherInfo.HauntedPlaces,
									SelfIntro:          catcherInfo.SelfIntro,
									CoverURL:           catcherInfo.CoverURL,
									GroupID:            groupID,
									GroupName:          ownGroupNames[idx],
								})
							}

							for _, catcher := range finalCatchers {
								if _, err := catcherRepo.Create(catcher); err != nil {
									log.Println(err)
									return
								}
							}

							if _, err := bot.ReplyMessage(event.ReplyToken,
								linebot.NewTextMessage("抓抓樂資料已更新完成"),
								linebot.NewFlexMessage("抓抓樂資訊", &linebot.CarouselContainer{
									Type:     linebot.FlexContainerTypeCarousel,
									Contents: makeCatcherContents(finalCatchers),
								})).Do(); err != nil {
								log.Println(err)
							}
						}
					}
				}
			}
		} else {
			groupID := event.Source.GroupID
			log.Printf("group id: %s", groupID)

			if event.Type == linebot.EventTypeMemberJoined {
				if _, ok := allGroupIDs[groupID]; ok {
					names := make([]string, 0)
					for _, member := range event.Members {
						userID := member.UserID
						log.Printf("user id: %s", userID)
						if profile, err := bot.GetGroupMemberProfile(groupID, userID).Do(); err != nil {
							log.Println(err)
						} else {
							names = append(names, profile.DisplayName)
						}
					}

					welcome(event.ReplyToken, strings.Join(names, ","))
				}
			} else if event.Type == linebot.EventTypeMessage {
				switch message := event.Message.(type) {
				case *linebot.TextMessage:
					//log.Printf("group id: %s, msg: %s", groupID, message.Text)

					if !strings.HasPrefix(message.Text, "?") &&
						!strings.HasSuffix(message.Text, "?") &&
						!strings.HasPrefix(message.Text, "？") &&
						!strings.HasSuffix(message.Text, "？") {
						return
					}

					msg := strings.TrimPrefix(message.Text, "?")
					msg = strings.TrimPrefix(msg, "？")
					msg = strings.TrimSuffix(msg, "?")
					msg = strings.TrimSuffix(msg, "？")

					switch msg {
					case "test welcome":
						welcome(event.ReplyToken, "test")
					case "指令", "常用指令":
						reply(event.ReplyToken, message.Text,
							linebot.NewMessageAction("交車", "交車？"),
							linebot.NewMessageAction("外觀", "外觀相關？"),
							linebot.NewMessageAction("內裝", "內裝相關？"),
							linebot.NewMessageAction("設定", "設定相關？"),
							linebot.NewMessageAction("行車記錄器", "行車記錄器？"),
							linebot.NewMessageAction("輪胎", "輪胎相關？"),
							linebot.NewMessageAction("防跳石網", "防跳石網？"),
							linebot.NewMessageAction("鑰匙皮套", "鑰匙皮套？"),
							linebot.NewMessageAction("遮陽簾", "遮陽簾？"),
							linebot.NewMessageAction("隔熱紙", "隔熱紙？"),
							linebot.NewURIAction("更多 (尚未更新)", "https://drive.google.com/file/d/1AM7PAPzMhp9BT3qKEP0lMdDKEx62kRSW/view"),
						)
					case "交車":
						reply(event.ReplyToken, message.Text,
							linebot.NewURIAction("交車前驗車檢查項目2.0", "https://drive.google.com/file/d/19N6rUajn42eWfQJMikYySdcyGEvr1QR4/view"),
							linebot.NewURIAction("正式交車檢查2.0", "https://drive.google.com/file/d/1S-XPfwNZFWAwQzc3gZbOj3vM8dP7TXR4/view"),
						)
					case "族貼", "族框":
						reply(event.ReplyToken, message.Text,
							linebot.NewURIAction("KAMIQ TW CLUB 族貼 | 族框", "https://kamiq.club/article?sid=350&aid=434"),
						)
					case "外觀相關":
						reply(event.ReplyToken, message.Text,
							linebot.NewURIAction("水簾洞與導水條", "https://kamiq.club/article?sid=324&aid=378"),
							linebot.NewURIAction("雨刷異音、會跳、立雨刷與更換", "https://kamiq.club/article?sid=324&aid=379"),
							linebot.NewURIAction("後視鏡指甲倒插問題", "https://kamiq.club/article?sid=324&aid=381"),
							linebot.NewURIAction("第三煞車燈水氣無法散去", "https://kamiq.club/article?sid=324&aid=382"),
							linebot.NewURIAction("更多", "https://kamiq.club/article?sid=324"),
						)
					case "內裝相關":
						reply(event.ReplyToken, message.Text,
							linebot.NewURIAction("車室異音-低速篇", "https://kamiq.club/article?sid=325&aid=383"),
							linebot.NewURIAction("車室異音-高速篇", "https://kamiq.club/article?sid=325&aid=384"),
							linebot.NewURIAction("車室靜音工程(含DIY與外廠安裝)", "https://kamiq.club/article?sid=325&aid=386"),
							linebot.NewURIAction("冷氣濾網更換", "https://kamiq.club/article?sid=325&aid=400"),
							linebot.NewURIAction("更多", "https://kamiq.club/article?sid=325"),
						)
					case "設定相關":
						reply(event.ReplyToken, message.Text,
							linebot.NewURIAction("搖控器啟閉車窗示範", "https://kamiq.club/article?sid=328&aid=375"),
							linebot.NewURIAction("Keyless鑰匙沒電手動開門方式", "https://kamiq.club/article?sid=328&aid=376"),
							linebot.NewURIAction("怠速引擎熄火判斷條件", "https://kamiq.club/article?sid=328&aid=377"),
							linebot.NewURIAction("更多", "https://kamiq.club/article?sid=328"),
						)
					case "行車記錄器":
						reply(event.ReplyToken, message.Text,
							linebot.NewURIAction("Garmin 66WD", "https://kamiq.club/article?sid=329&aid=394"),
							linebot.NewURIAction("HP S970 (電子後視鏡)", "https://kamiq.club/article?sid=329&aid=395"),
							linebot.NewURIAction("DOD RX900", "https://kamiq.club/article?sid=329&aid=503"),
							linebot.NewURIAction("更多", "https://kamiq.club/article?sid=328"),
						)
					case "輪胎相關":
						reply(event.ReplyToken, message.Text,
							linebot.NewURIAction("胎壓偵測器", "https://kamiq.club/article?sid=334&aid=388"),
							linebot.NewURIAction("有線/無線打氣機", "https://kamiq.club/article?sid=334&aid=456"),
							linebot.NewURIAction("更多", "https://kamiq.club/article?sid=334"),
						)
					case "防跳石網":
						reply(event.ReplyToken, message.Text,
							linebot.NewURIAction("防跳石網安裝", "https://kamiq.club/article?sid=335&aid=402"),
							linebot.NewURIAction("防跳石網配色參考", "https://kamiq.club/article?sid=335&aid=404"),
							linebot.NewURIAction("怠速引擎熄火判斷條件", "https://kamiq.club/article?sid=328&aid=377"),
							linebot.NewURIAction("更多", "https://kamiq.club/article?sid=335"),
						)
					case "鑰匙皮套":
						reply(event.ReplyToken, message.Text,
							linebot.NewURIAction("Hsu's 頑皮革", "https://kamiq.club/article?sid=338&aid=416"),
							linebot.NewURIAction("Story Leather", "https://kamiq.club/article?sid=338&aid=425"),
							linebot.NewURIAction("賽頓精品手工皮件", "https://kamiq.club/article?sid=338&aid=423"),
							linebot.NewURIAction("JC手作客製皮套", "https://kamiq.club/article?sid=338&aid=424"),
							linebot.NewURIAction("更多", "https://kamiq.club/article?sid=338"),
						)
					case "遮陽簾":
						reply(event.ReplyToken, message.Text,
							linebot.NewURIAction("晴天遮陽簾", "https://kamiq.club/article?sid=330&aid=438"),
							linebot.NewURIAction("徐府遮陽簾", "https://kamiq.club/article?sid=330&aid=439"),
							linebot.NewURIAction("更多", "https://kamiq.club/article?sid=330"),
						)
					case "隔熱紙":
						reply(event.ReplyToken, message.Text,
							linebot.NewURIAction("GAMA-E系列", "https://kamiq.club/article?sid=330&aid=403"),
							linebot.NewURIAction("Carlife X系列", "https://kamiq.club/article?sid=330&aid=417"),
							linebot.NewURIAction("3M極黑系列", "https://kamiq.club/article?sid=330&aid=499"),
							linebot.NewURIAction("Solar Gard 舒熱佳鑽石 LX 系列", "https://kamiq.club/article?sid=330&aid=500"),
							linebot.NewURIAction("更多", "https://kamiq.club/article?sid=330"),
						)
					case "避光墊":
						reply(event.ReplyToken, message.Text,
							linebot.NewURIAction("愛力美奈納碳避光墊", "https://kamiq.club/article?sid=333&aid=427"),
							linebot.NewURIAction("BSM專用仿麂皮避光墊", "https://kamiq.club/article?sid=333&aid=428"),
						)
					case "晴雨窗":
						reply(event.ReplyToken, message.Text,
							linebot.NewURIAction("晴雨窗", "https://kamiq.club/article?sid=333&aid=445"),
						)
					case "腳踏墊":
						reply(event.ReplyToken, message.Text,
							linebot.NewURIAction("3D卡固", "https://kamiq.club/article?sid=331&aid=406"),
							linebot.NewURIAction("Škoda原廠腳踏墊", "https://kamiq.club/article?sid=331&aid=420"),
							linebot.NewURIAction("台中裕峰訂製款", "https://kamiq.club/article?sid=331&aid=419"),
						)
					case "後車廂墊":
						reply(event.ReplyToken, message.Text,
							linebot.NewURIAction("後車廂墊", "https://kamiq.club/article?sid=331&aid=430"),
							linebot.NewURIAction("3M安美", "https://kamiq.club/article?sid=331&aid=418"),
						)
					case "車側飾板", "後廂護板":
						reply(event.ReplyToken, message.Text,
							linebot.NewURIAction("車側飾板|後廂護板", "https://kamiq.club/article?sid=336"),
						)
					case "其他週邊":
						reply(event.ReplyToken, message.Text,
							linebot.NewURIAction("旋轉杯架", "https://kamiq.club/article?sid=350&aid=436"),
							linebot.NewURIAction("後行李箱連動燈", "https://kamiq.club/article?sid=350&aid=448"),
							linebot.NewURIAction("光控燈膜", "https://kamiq.club/article?sid=350&aid=446"),
							linebot.NewURIAction("KAMIQ TW CLUB 族貼 | 族框", "https://kamiq.club/article?sid=350&aid=434"),
							linebot.NewURIAction("更多", "https://kamiq.club/article?sid=350"),
						)
					case "原廠週邊":
						reply(event.ReplyToken, message.Text,
							linebot.NewURIAction("原廠週邊價格表", "https://kamiq.club/article?sid=349&aid=407"),
							linebot.NewURIAction("原廠檔泥板", "https://kamiq.club/article?sid=349&aid=444"),
							linebot.NewURIAction("原廠門側垃圾桶", "https://kamiq.club/article?sid=349&aid=442"),
							linebot.NewURIAction("原廠多媒體底座", "https://kamiq.club/article?sid=349&aid=443"),
							linebot.NewURIAction("更多", "https://kamiq.club/article?sid=349"),
						)
					default:
						if num, err := strconv.Atoi(msg); err == nil && num < 10000 && len(msg) == 4 {
							catchers, _ := catcherRepo.SearchByLicensePlateNumber(groupID, msg)
							if len(catchers) > 0 {
								if _, err := bot.ReplyMessage(event.ReplyToken, linebot.NewFlexMessage("抓抓樂資訊", &linebot.CarouselContainer{
									Type:     linebot.FlexContainerTypeCarousel,
									Contents: makeCatcherContents(catchers),
								})).Do(); err != nil {
									log.Println(err)
								}
							} else {
								cnt, err := catcherRepo.IncreaseWildCatcher(msg)
								if err != nil {
									log.Println(err)
									return
								}
								if _, err := bot.ReplyMessage(event.ReplyToken, linebot.NewTextMessage(fmt.Sprintf("捕獲野生卡米!!\n趕快收服牠吧!!\n目前該車號已被發現 %d 次", cnt))).Do(); err != nil {
									log.Println(err)
								}
							}
						}
					}
				}
			}
		}
	}
}

func welcome(replyToken, names string) {
	if names != "" {
		names = fmt.Sprintf(" %s ", names)
	}
	if _, err := bot.ReplyMessage(replyToken, linebot.NewTextMessage(fmt.Sprintf(
		`新朋友`+names+`您好!!
歡迎加入KamiQ車主限定群

有任何問題可於
官網查詢、詢問機器人
或直接發問哦~
群組訊息較多，記得關提醒!!

以下連結請務必看一下哦~`)),
		linebot.NewFlexMessage("資訊卡", &linebot.CarouselContainer{
			Type:     linebot.FlexContainerTypeCarousel,
			Contents: makeInfoCard(),
		})).Do(); err != nil {
		log.Print(err)
	}
}

func makeCatcherContents(catchers []repositories.Catcher) []*linebot.BubbleContainer {
	result := make([]*linebot.BubbleContainer, 0)
	finalCatchers := map[string]*repositories.Catcher{}

	for _, catcher := range catchers {
		catcher := catcher
		if finalCatcher, ok := finalCatchers[catcher.LicensePlateNumber]; ok {
			finalCatchers[catcher.LicensePlateNumber].GroupName = finalCatcher.GroupName + "/" + catcher.GroupName
		} else {
			finalCatchers[catcher.LicensePlateNumber] = &catcher
		}
	}

	for _, catcher := range finalCatchers {
		flex1 := 1
		flex2 := 2
		carNumber := make([]linebot.FlexComponent, 0)
		carNumber = append(carNumber, &linebot.IconComponent{
			URL: "https://scdn.line-apps.com/n/channel_devcenter/img/fx/review_gold_star_28.png",
		})
		carNumber = append(carNumber, &linebot.TextComponent{
			Color: "#aaaaaa",
			Size:  linebot.FlexTextSizeTypeMd,
			Text:  "車牌號碼:",
			Flex:  &flex1,
		})
		carNumber = append(carNumber, &linebot.TextComponent{
			Color: "#666666",
			Size:  linebot.FlexTextSizeTypeMd,
			Text:  catcher.LicensePlateNumber,
			Flex:  &flex2,
			Wrap:  true,
		})
		lineID := make([]linebot.FlexComponent, 0)
		lineID = append(lineID, &linebot.IconComponent{
			URL: "https://scdn.line-apps.com/n/channel_devcenter/img/fx/review_gold_star_28.png",
		})
		lineID = append(lineID, &linebot.TextComponent{
			Color: "#aaaaaa",
			Size:  linebot.FlexTextSizeTypeMd,
			Text:  "賴的名稱:",
			Flex:  &flex1,
		})
		lineID = append(lineID, &linebot.TextComponent{
			Color: "#666666",
			Size:  linebot.FlexTextSizeTypeMd,
			Text:  catcher.UserName,
			Flex:  &flex2,
			Wrap:  true,
		})
		place := make([]linebot.FlexComponent, 0)
		place = append(place, &linebot.IconComponent{
			URL: "https://scdn.line-apps.com/n/channel_devcenter/img/fx/review_gold_star_28.png",
		})
		place = append(place, &linebot.TextComponent{
			Color: "#aaaaaa",
			Size:  linebot.FlexTextSizeTypeMd,
			Text:  "出沒地點:",
			Flex:  &flex1,
		})
		place = append(place, &linebot.TextComponent{
			Color: "#666666",
			Size:  linebot.FlexTextSizeTypeMd,
			Text:  catcher.HauntedPlaces,
			Flex:  &flex2,
			Wrap:  true,
		})
		group := make([]linebot.FlexComponent, 0)
		group = append(group, &linebot.IconComponent{
			URL: "https://scdn.line-apps.com/n/channel_devcenter/img/fx/review_gold_star_28.png",
		})
		group = append(group, &linebot.TextComponent{
			Color: "#aaaaaa",
			Size:  linebot.FlexTextSizeTypeMd,
			Text:  "所在群組:",
			Flex:  &flex1,
		})
		group = append(group, &linebot.TextComponent{
			Color: "#666666",
			Size:  linebot.FlexTextSizeTypeMd,
			Text:  catcher.GroupName,
			Flex:  &flex2,
			Wrap:  true,
		})

		intro := make([]linebot.FlexComponent, 0)
		intro = append(intro, &linebot.IconComponent{
			URL: "https://scdn.line-apps.com/n/channel_devcenter/img/fx/review_gold_star_28.png",
		})
		intro = append(intro, &linebot.TextComponent{
			Color: "#aaaaaa",
			Size:  linebot.FlexTextSizeTypeMd,
			Text:  "自我介紹:",
			Flex:  &flex1,
		})
		intro = append(intro, &linebot.TextComponent{
			Color: "#666666",
			Size:  linebot.FlexTextSizeTypeMd,
			Text:  catcher.SelfIntro,
			Flex:  &flex2,
			Wrap:  true,
		})

		components := make([]linebot.FlexComponent, 0)
		components = append(components, &linebot.BoxComponent{
			Layout:   linebot.FlexBoxLayoutTypeBaseline,
			Spacing:  linebot.FlexComponentSpacingTypeSm,
			Contents: carNumber,
		})
		components = append(components, &linebot.BoxComponent{
			Layout:   linebot.FlexBoxLayoutTypeBaseline,
			Spacing:  linebot.FlexComponentSpacingTypeSm,
			Contents: lineID,
		})
		components = append(components, &linebot.BoxComponent{
			Layout:   linebot.FlexBoxLayoutTypeBaseline,
			Spacing:  linebot.FlexComponentSpacingTypeSm,
			Contents: place,
		})
		components = append(components, &linebot.BoxComponent{
			Layout:   linebot.FlexBoxLayoutTypeBaseline,
			Spacing:  linebot.FlexComponentSpacingTypeSm,
			Contents: group,
		})
		components = append(components, &linebot.BoxComponent{
			Layout:   linebot.FlexBoxLayoutTypeBaseline,
			Spacing:  linebot.FlexComponentSpacingTypeSm,
			Contents: intro,
		})

		result = append(result, &linebot.BubbleContainer{
			Type: linebot.FlexContainerTypeBubble,
			Hero: &linebot.ImageComponent{
				Type:        linebot.FlexComponentTypeImage,
				URL:         catcher.CoverURL,
				Size:        linebot.FlexImageSizeTypeFull,
				AspectRatio: linebot.FlexImageAspectRatioType20to13,
				AspectMode:  linebot.FlexImageAspectModeTypeCover,
			},
			Body: &linebot.BoxComponent{
				Type:     linebot.FlexComponentTypeBox,
				Layout:   linebot.FlexBoxLayoutTypeVertical,
				Contents: components,
			},
		})
	}

	return result
}

func makeInfoCard() []*linebot.BubbleContainer {
	contents := make([]*linebot.BubbleContainer, 0)
	newsComponent := make([]linebot.FlexComponent, 0)
	newsComponent = append(newsComponent, &linebot.ButtonComponent{
		Type:   linebot.FlexComponentTypeButton,
		Action: linebot.NewURIAction("入群必讀", "https://kamiq.club/news?hid=498&nid=214"),
		Style:  linebot.FlexButtonStyleTypePrimary,
	})
	siteComponent := make([]linebot.FlexComponent, 0)
	siteComponent = append(siteComponent, &linebot.ButtonComponent{
		Type:   linebot.FlexComponentTypeButton,
		Action: linebot.NewURIAction("KamiQ車友群官網", "https://kamiq.club"),
		Style:  linebot.FlexButtonStyleTypePrimary,
	})
	catcherComponent := make([]linebot.FlexComponent, 0)
	catcherComponent = append(catcherComponent, &linebot.ButtonComponent{
		Type:   linebot.FlexComponentTypeButton,
		Action: linebot.NewURIAction("一起抓抓樂", "https://lin.ee/e6uqqPo"),
		Style:  linebot.FlexButtonStyleTypePrimary,
	})
	contents = append(contents, &linebot.BubbleContainer{
		Type: linebot.FlexContainerTypeBubble,
		Hero: &linebot.ImageComponent{
			Type:        linebot.FlexComponentTypeImage,
			URL:         "https://kamiq.club/upload/36/news_images/6b8a6da0-cafb-4904-87b7-d9ffa01b2075.jpeg",
			Size:        linebot.FlexImageSizeTypeFull,
			AspectRatio: linebot.FlexImageAspectRatioType20to13,
			AspectMode:  linebot.FlexImageAspectModeTypeCover,
		},
		Footer: &linebot.BoxComponent{
			Type:     linebot.FlexComponentTypeButton,
			Layout:   linebot.FlexBoxLayoutTypeVertical,
			Contents: newsComponent,
		},
	})
	contents = append(contents, &linebot.BubbleContainer{
		Type: linebot.FlexContainerTypeBubble,
		Hero: &linebot.ImageComponent{
			Type:        linebot.FlexComponentTypeImage,
			URL:         "https://i.imgur.com/Jo0JBxU.png",
			Size:        linebot.FlexImageSizeTypeFull,
			AspectRatio: linebot.FlexImageAspectRatioType20to13,
			AspectMode:  linebot.FlexImageAspectModeTypeCover,
		},
		Footer: &linebot.BoxComponent{
			Type:     linebot.FlexComponentTypeButton,
			Layout:   linebot.FlexBoxLayoutTypeVertical,
			Contents: siteComponent,
		},
	})
	contents = append(contents, &linebot.BubbleContainer{
		Type: linebot.FlexContainerTypeBubble,
		Hero: &linebot.ImageComponent{
			Type:        linebot.FlexComponentTypeImage,
			URL:         "https://i.imgur.com/rILuNbA.jpg",
			Size:        linebot.FlexImageSizeTypeFull,
			AspectRatio: linebot.FlexImageAspectRatioType20to13,
			AspectMode:  linebot.FlexImageAspectModeTypeCover,
		},
		Footer: &linebot.BoxComponent{
			Type:     linebot.FlexComponentTypeButton,
			Layout:   linebot.FlexBoxLayoutTypeVertical,
			Contents: catcherComponent,
		},
	})
	return contents
}

func reply(replyToken, msg string, actions ...linebot.TemplateAction) {
	contents := make([]*linebot.BubbleContainer, 0, len(actions))
	for _, act := range actions {
		btnComponent := make([]linebot.FlexComponent, 0)
		btnComponent = append(btnComponent, &linebot.ButtonComponent{
			Type:   linebot.FlexComponentTypeButton,
			Action: act,
			Style:  linebot.FlexButtonStyleTypePrimary,
			//Color:  "#8E8E8E",
		})
		contents = append(contents, &linebot.BubbleContainer{
			Type: linebot.FlexContainerTypeBubble,
			Hero: &linebot.ImageComponent{
				Type:        linebot.FlexComponentTypeImage,
				URL:         "https://kamiq.club/upload/36/favicon_images/c1a630ef-c78f-43cc-b95e-0619f3f4da4d.jpg",
				Size:        linebot.FlexImageSizeTypeFull,
				AspectRatio: linebot.FlexImageAspectRatioType20to13,
				AspectMode:  linebot.FlexImageAspectModeTypeFit,
			},
			Footer: &linebot.BoxComponent{
				Type:     linebot.FlexComponentTypeButton,
				Layout:   linebot.FlexBoxLayoutTypeVertical,
				Contents: btnComponent,
			},
		})
	}

	if _, err := bot.ReplyMessage(replyToken, linebot.NewFlexMessage(msg, &linebot.CarouselContainer{
		Type:     linebot.FlexContainerTypeCarousel,
		Contents: contents,
	})).Do(); err != nil {
		log.Println(err)
	}
}

func uploadImgur(data io.ReadCloser) string {
	defer data.Close()
	content, err := ioutil.ReadAll(data)
	if err != nil {
		log.Println(err)
		return ""
	}

	url := "https://api.imgur.com/3/image"
	method := "POST"

	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	_ = writer.WriteField("image", string(content))
	err = writer.Close()
	if err != nil {
		log.Println(err)
		return ""
	}

	client := &http.Client{}
	req, err := http.NewRequest(method, url, payload)

	if err != nil {
		log.Println(err)
		return ""
	}
	req.Header.Add("Authorization", fmt.Sprintf("Client-ID %s", imgurClientID))

	req.Header.Set("Content-Type", writer.FormDataContentType())
	res, err := client.Do(req)
	if err != nil {
		log.Println(err)
		return ""
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Println(err)
		return ""
	}

	resp := ImgurResp{}
	if err := json.Unmarshal(body, &resp); err != nil {
		log.Println(err)
		return ""
	}

	return resp.Data.Link
}
