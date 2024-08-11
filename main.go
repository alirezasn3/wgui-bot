package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Peer struct {
	ID                   string `json:"_id" bson:"_id"`
	Name                 string `json:"name" bson:"name"`
	PublicKey            string `json:"publicKey" bson:"publicKey"`
	AllowedIPs           string `json:"allowedIPs" bson:"allowedIPs"`
	Disabled             bool   `json:"disabled" bson:"disabled"`
	AllowedUsage         int64  `json:"allowedUsage" bson:"allowedUsage"`
	ExpiresAt            int64  `json:"expiresAt" bson:"expiresAt"`
	TotalTX              int64  `json:"totalTX" bson:"totalTX"`
	TotalRX              int64  `json:"totalRX" bson:"totalRX"`
	TelegramChatID       int64
	ReceivedExpiryNotice bool
	ReceivedUsageNotice  bool
}

type Config struct {
	MongoURI         string `json:"mongoURI"`
	DBName           string `json:"dbName"`
	CollectionName   string `json:"collectionName"`
	TelegramBotToken string `json:"telegramBotToken"`
	ChannelID        string `json:"channelID"`
	AdminID          string `json:"adminID"`
}

var bot *tgbotapi.BotAPI
var collection *mongo.Collection
var config *Config

// formatBytes formats the byte size in a human-readable format
func formatBytes(totalBytes int64, space bool) string {
	if totalBytes == 0 {
		if space {
			return "00.00 KB"
		} else {
			return "00.00KB"
		}
	}

	var totalKilos float64 = float64(totalBytes / 1024)
	var totalMegas float64 = totalKilos / 1000
	var totalGigas float64 = totalMegas / 1000
	var totalTeras float64 = totalGigas / 1000

	var unit string
	var value float64
	switch {
	case totalKilos < 100:
		unit = "KB"
		value = float64(totalKilos)
	case totalMegas < 100:
		unit = "MB"
		value = float64(totalMegas)
	case totalGigas < 100:
		unit = "GB"
		value = float64(totalGigas)
	default:
		unit = "TB"
		value = float64(totalTeras)
	}

	if space {
		if value == math.Trunc(value) {
			return fmt.Sprintf("%.0f %s", value, unit)
		} else {
			return fmt.Sprintf("%.1f %s", value, unit)
		}
	} else {
		if value == math.Trunc(value) {
			return fmt.Sprintf("%.0f%s", value, unit)
		} else {
			return fmt.Sprintf("%.1f%s", value, unit)
		}
	}
}

// formatExpiry formats the expiry time in a human-readable format
func formatExpiry(expiresAt int64, noPrefix bool) string {
	if expiresAt == 0 {
		return "unknown"
	}

	totalSeconds := math.Trunc(float64(expiresAt-time.Now().UnixMilli()) / 1000)
	prefix := ""
	if totalSeconds < 0 && !noPrefix {
		prefix = "-"
	}
	totalSeconds = math.Abs(totalSeconds)

	if totalSeconds < 60 {
		return fmt.Sprintf("%s%.0fseconds", prefix, totalSeconds)
	}

	totalMinutes := math.Trunc(totalSeconds / 60)
	if totalMinutes < 60 {
		return fmt.Sprintf("%s%.0fminutes", prefix, totalMinutes)
	}

	totalHours := math.Trunc(totalMinutes / 60)
	if totalHours < 24 {
		return fmt.Sprintf("%s%.0fhours", prefix, totalHours)
	}

	return fmt.Sprintf("%s%.0fdays", prefix, math.Trunc(totalHours/24))
}

func init() {
	execPath, err := os.Executable()
	if err != nil {
		panic(err)
	}
	path := filepath.Dir(execPath)
	bytes, err := os.ReadFile(filepath.Join(path, "config.json"))
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(bytes, &config)
	if err != nil {
		panic(err)
	}
	log.Println("Loaded config from " + filepath.Join(path, "config.json"))
	bot, err = tgbotapi.NewBotAPI(config.TelegramBotToken)
	if err != nil {
		panic(err)
	}
	webhookinfo, err := bot.GetWebhookInfo()
	if err != nil {
		panic(err)
	}
	if webhookinfo.URL != "" {
		log.Println("deleting webhook: " + webhookinfo.URL)
		res, err := bot.Request(tgbotapi.DeleteWebhookConfig{DropPendingUpdates: true})
		if err != nil {
			panic(err)
		}
		if !res.Ok {
			panic(res.Description)
		}
	}
	log.Printf("telegram bot username: %s\n", bot.Self.UserName)
	mongoClient, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(config.MongoURI).SetServerAPIOptions(options.ServerAPI(options.ServerAPIVersion1)))
	if err != nil {
		panic(err)
	}
	collection = mongoClient.Database(config.DBName).Collection(config.CollectionName)
	log.Println("Connected to database")
}

func main() {

	go func() {
		u := tgbotapi.NewUpdate(0)
		u.Timeout = 60
		updates := bot.GetUpdatesChan(u)
		for update := range updates {
			if update.Message != nil {
				if update.Message.Command() == "list" {
					var peers []Peer
					cursor, err := collection.Find(context.TODO(), bson.M{"telegramChatID": update.Message.From.ID})
					if err != nil {
						panic(err)
					}
					if err = cursor.All(context.TODO(), &peers); err != nil {
						panic(err)
					}
					if len(peers) == 0 {
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, "هیج کاربری ثبت نشده است")
						msg.ReplyToMessageID = update.Message.MessageID
						_, err := bot.Send(msg)
						if err != nil {
							log.Println(err)
						}
					}
					text := ""
					for _, p := range peers {
						text += fmt.Sprintf("<b>%s</b>\n\t-> %s/%s %s\n\n", p.Name, formatBytes(p.TotalRX+p.TotalTX, false), formatBytes(p.AllowedUsage, false), formatExpiry(p.ExpiresAt-time.Now().Unix(), false))
					}
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)
					msg.ReplyToMessageID = update.Message.MessageID
					msg.ParseMode = tgbotapi.ModeHTML
					_, err = bot.Send(msg)
					if err != nil {
						log.Println(err)
					}
				} else if update.Message.CommandArguments() != "" {
					arg, err := base64.StdEncoding.DecodeString(update.Message.CommandArguments())
					if err != nil {
						log.Println(err)
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, "درخواست نامعتبر")
						msg.ReplyToMessageID = update.Message.MessageID
						_, err = bot.Send(msg)
						if err != nil {
							log.Println(err)
						}
						continue
					}
					p := Peer{}
					err = collection.FindOne(context.Background(), bson.M{"publicKey": string(arg)}).Decode(&p)
					if err != nil {
						log.Println(err)
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, "درخواست نامعتبر")
						msg.ReplyToMessageID = update.Message.MessageID
						_, err = bot.Send(msg)
						if err != nil {
							log.Println(err)
						}
						continue
					}
					_, err = collection.UpdateOne(context.TODO(), bson.M{"publicKey": string(arg)}, bson.M{"$set": bson.M{"telegramChatID": update.Message.From.ID}})
					if err != nil {
						log.Println(err)
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, "درخواست نامعتبر")
						msg.ReplyToMessageID = update.Message.MessageID
						_, err = bot.Send(msg)
						if err != nil {
							log.Println(err)
						}
						continue
					}
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("اشتراک <b>%s</b> ثبت شد.\n\n<a href=\"https://t.me/%s\">مشاهده کانال</a>", p.Name, config.ChannelID))
					msg.ReplyToMessageID = update.Message.MessageID
					msg.ParseMode = tgbotapi.ModeHTML
					_, err = bot.Send(msg)
					if err != nil {
						log.Println(err)
					}
				} else {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "درخواست نامعتبر")
					msg.ReplyToMessageID = update.Message.MessageID
					_, err := bot.Send(msg)
					if err != nil {
						log.Println(err)
					}
				}
			}
		}
	}()

	for {
		t := time.Now().UnixMilli()
		var peers []*Peer
		cursor, err := collection.Find(context.TODO(), bson.D{})
		if err != nil {
			panic(err)
		}
		if err = cursor.All(context.TODO(), &peers); err != nil {
			panic(err)
		}
		for _, p := range peers {
			if p.TelegramChatID == 0 || p.Disabled {
				continue
			}
			if !p.ReceivedUsageNotice && p.AllowedUsage-(p.TotalRX+p.TotalTX) < 1024000000 {
				msg := tgbotapi.NewMessage(p.TelegramChatID, fmt.Sprintf("ترافیک قابل استفاده اشتراک <b>%s</b> کمتر از یک گیگابایت است.\n\n<a href=\"https://t.me/%s\">تمدید اشتراک</a>", p.Name, config.AdminID))
				msg.ParseMode = tgbotapi.ModeHTML
				_, err = bot.Send(msg)
				if err != nil {
					log.Println(err)
					continue
				}
				_, err = collection.UpdateOne(context.TODO(), bson.M{"publicKey": p.PublicKey}, bson.M{"$set": bson.M{"receivedUsageNotice": true}})
				if err != nil {
					log.Println(err)
					continue
				}
			} else if p.ReceivedUsageNotice && p.AllowedUsage-(p.TotalRX+p.TotalTX) > 1024000000 {
				_, err = collection.UpdateOne(context.TODO(), bson.M{"publicKey": p.PublicKey}, bson.M{"$set": bson.M{"receivedUsageNotice": false}})
				if err != nil {
					log.Println(err)
					continue
				}
			} else if !p.ReceivedExpiryNotice && p.ExpiresAt-t < 86400000 {
				msg := tgbotapi.NewMessage(p.TelegramChatID, fmt.Sprintf("اشتراک <b>%s</b> کمتر از ۲۴ ساعت دیگر به پایان میرسد.\n\n<a href=\"https://t.me/%s\">تمدید اشتراک</a>", p.Name, config.AdminID))
				msg.ParseMode = tgbotapi.ModeHTML
				_, err = bot.Send(msg)
				if err != nil {
					log.Println(err)
					continue
				}
				_, err = collection.UpdateOne(context.TODO(), bson.M{"publicKey": p.PublicKey}, bson.M{"$set": bson.M{"receivedExpiryNotice": true}})
				if err != nil {
					log.Println(err)
					continue
				}
			} else if p.ReceivedExpiryNotice && p.ExpiresAt-t > 86400000 {
				_, err = collection.UpdateOne(context.TODO(), bson.M{"publicKey": p.PublicKey}, bson.M{"$set": bson.M{"receivedExpiryNotice": false}})
				if err != nil {
					log.Println(err)
					continue
				}
			}
		}

		time.Sleep(time.Second * 3)
	}
}
