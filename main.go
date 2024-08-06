package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
		// var err error
		u := tgbotapi.NewUpdate(0)
		u.Timeout = 60
		updates := bot.GetUpdatesChan(u)
		for update := range updates {
			if update.Message != nil {
				// check if message is command
				if update.Message.Command() != "" {
					if update.Message.Command() == "start" {
						tt := update.Message.CommandArguments()
						// check if arg is peer's telegram token
						if len(tt) == 44 {
							p := Peer{}
							err := collection.FindOne(context.Background(), bson.M{"publicKey": tt}).Decode(&p)
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
							_, err = collection.UpdateOne(context.TODO(), bson.M{"publicKey": tt}, bson.M{"$set": bson.M{"telegramChatID": update.Message.From.ID}})
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
						}
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
