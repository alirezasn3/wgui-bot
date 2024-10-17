package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
	goSystemd "github.com/alirezasn3/go-systemd"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Peer struct {
	ID                   string             `json:"ID" bson:"_id"`
	Name                 string             `json:"Name" bson:"name"`
	PublicKey            string             `json:"PublicKey" bson:"publicKey"`
	PrivateKey           string             `json:"PrivateKey" bson:"privateKey"`
	AllowedIPs           string             `json:"AllowedIPs" bson:"allowedIPs"`
	Disabled             bool               `json:"Disabled" bson:"disabled"`
	AllowedUsage         int64              `json:"AllowedUsage" bson:"allowedUsage"`
	ExpiresAt            int64              `json:"ExpiresAt" bson:"expiresAt"`
	TotalTX              int64              `json:"TotalTX" bson:"totalTX"`
	TotalRX              int64              `json:"TotalRX" bson:"totalRX"`
	TelegramChatID       int64              `json:"TelegramChatID" bson:"telegramChatID"`
	ReceivedExpiryNotice bool               `json:"ReceivedExpiryNotice" bson:"receivedExpiryNotice"`
	ReceivedUsageNotice  bool               `json:"ReceivedUsageNotice" bson:"receivedUsageNotice"`
	GroupID              primitive.ObjectID `json:"GroupID" bson:"groupID"`
}

type Group struct {
	ID           primitive.ObjectID `json:"ID" bson:"_id"`
	Name         string             `json:"Name" bson:"name"`
	PeerIDs      []string           `json:"PeerIDs" bson:"peerIDs"`
	AllowedUsage int64              `json:"AllowedUsage" bson:"allowedUsage"`
	TotalTX      int64              `json:"TotalTX" bson:"totalTX"`
	TotalRX      int64              `json:"TotalRX" bson:"totalRX"`
	ExpiresAt    int64              `json:"ExpiresAt" bson:"expiresAt"`
	Disabled     bool               `json:"Disabled" bson:"disabled"`
	OwnerID      string             `json:"OwnerID" bson:"ownerID"`
}

type Config struct {
	MongoURI         string `json:"mongoURI"`
	DBName           string `json:"dbName"`
	TelegramBotToken string `json:"telegramBotToken"`
	ChannelID        string `json:"channelID"`
	AdminID          string `json:"adminID"`
	ServerPublicKey  string `json:"serverPublicKey"`
	Endpoint         string `json:"endpoint"`
	BypassKey        string `json:"bypassKey"`
}

var b *gotgbot.Bot
var peersCollection *mongo.Collection
var groupsCollection *mongo.Collection
var config *Config
var path string

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

	// check for install and uninstall commands
	if runtime.GOOS == "linux" {
		if slices.Contains(os.Args, "--install") {
			execPath, err := os.Executable()
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			err = goSystemd.CreateService(&goSystemd.Service{Name: "wgui-bot", ExecStart: execPath, Restart: "on-failure", RestartSec: "3s"})
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			} else {
				fmt.Println("wgui-bot service created")
				os.Exit(0)
			}
		} else if slices.Contains(os.Args, "--uninstall") {
			err := goSystemd.DeleteService("wgui-bot")
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			} else {
				fmt.Println("wgui-bot service deleted")
				os.Exit(0)
			}
		}
	}

	path = filepath.Dir(execPath)
	bytes, err := os.ReadFile(filepath.Join(path, "config.json"))
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(bytes, &config)
	if err != nil {
		panic(err)
	}
	log.Println("Loaded config from " + filepath.Join(path, "config.json"))
	b, err = gotgbot.NewBot(config.TelegramBotToken, &gotgbot.BotOpts{
		BotClient: &gotgbot.BaseBotClient{
			Client: http.Client{},
			DefaultRequestOpts: &gotgbot.RequestOpts{
				Timeout: gotgbot.DefaultTimeout,
				APIURL:  gotgbot.DefaultAPIURL,
			},
		},
	})
	if err != nil {
		panic("Failed to create new bot: " + err.Error())
	}
	mongoClient, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(config.MongoURI).SetServerAPIOptions(options.ServerAPI(options.ServerAPIVersion1)))
	if err != nil {
		panic(err)
	}
	peersCollection = mongoClient.Database(config.DBName).Collection("peers")
	groupsCollection = mongoClient.Database(config.DBName).Collection("groups")
	log.Println("Connected to database")
}

func main() {
	// Create updater and dispatcher.
	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		// If an error is returned by a handler, log it and continue going.
		Error: func(b *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
			log.Println("an error occurred while handling update:", err.Error())
			return ext.DispatcherActionNoop
		},
		MaxRoutines: ext.DefaultMaxRoutines,
	})
	updater := ext.NewUpdater(dispatcher, nil)

	dispatcher.AddHandler(handlers.NewCommand("start", func(b *gotgbot.Bot, ctx *ext.Context) error {
		if len(strings.Split(ctx.EffectiveMessage.Text, " ")) < 2 {
			_, err := ctx.EffectiveMessage.Reply(b, "درخواست نامعتبر", nil)
			if err != nil {
				return fmt.Errorf("failed to send message: %w", err)
			}
			return nil
		}
		arg, err := base64.StdEncoding.DecodeString(strings.Split(ctx.EffectiveMessage.Text, " ")[1])
		if err != nil {
			log.Println(err)
			_, err = ctx.EffectiveMessage.Reply(b, "درخواست نامعتبر", nil)
			if err != nil {
				return fmt.Errorf("failed to send message: %w", err)
			}
			return nil
		}
		p := Peer{}
		err = peersCollection.FindOne(context.Background(), bson.M{"publicKey": string(arg)}).Decode(&p)
		if err != nil {
			log.Println(err)
			_, err = ctx.EffectiveMessage.Reply(b, "درخواست نامعتبر", nil)
			if err != nil {
				return fmt.Errorf("failed to send message: %w", err)
			}
			return nil
		}
		_, err = peersCollection.UpdateOne(context.TODO(), bson.M{"publicKey": string(arg)}, bson.M{"$set": bson.M{"telegramChatID": ctx.EffectiveMessage.From.Id}})
		if err != nil {
			log.Println(err)
			_, err = ctx.EffectiveMessage.Reply(b, "درخواست نامعتبر", nil)
			if err != nil {
				return fmt.Errorf("failed to send message: %w", err)
			}
			return nil
		}
		_, err = ctx.EffectiveMessage.Reply(b, fmt.Sprintf("اشتراک <b>%s</b> ثبت شد.\n\n<a href=\"https://t.me/%s\">مشاهده کانال</a>", p.Name, config.ChannelID), &gotgbot.SendMessageOpts{ParseMode: "HTML"})
		if err != nil {
			return fmt.Errorf("failed to send message: %w", err)
		}
		return nil
	}))

	dispatcher.AddHandler(handlers.NewCommand("list", func(b *gotgbot.Bot, ctx *ext.Context) error {
		var peers []Peer
		cursor, err := peersCollection.Find(context.TODO(), bson.M{"telegramChatID": ctx.EffectiveMessage.From.Id})
		if err != nil {
			_, err := ctx.EffectiveMessage.Reply(b, "درخواست با خطا مواجه شد", nil)
			if err != nil {
				return fmt.Errorf("failed to send message: %w", err)
			}
			return nil
		}
		if err = cursor.All(context.TODO(), &peers); err != nil {
			_, err := ctx.EffectiveMessage.Reply(b, "درخواست با خطا مواجه شد", nil)
			if err != nil {
				return fmt.Errorf("failed to send message: %w", err)
			}
			return nil
		}
		if len(peers) == 0 {
			_, err = ctx.EffectiveMessage.Reply(b, "هیج کاربری ثبت نشده است", nil)
			if err != nil {
				return fmt.Errorf("failed to send message: %w", err)
			}
			return nil
		}
		text := ""
		var groups []primitive.ObjectID
		for _, p := range peers {
			if !p.GroupID.IsZero() {
				if !slices.Contains(groups, p.GroupID) {
					groups = append(groups, p.GroupID)
				}
				continue
			}
			text += fmt.Sprintf("┏ <b>%s</b>\n┣━ %s / %s\n┗━ %s\n\n", p.Name, formatBytes(p.TotalRX+p.TotalTX, true), formatBytes(p.AllowedUsage, true), formatExpiry(p.ExpiresAt, false))
		}
		for _, groupID := range groups {
			var g Group
			err = groupsCollection.FindOne(context.Background(), bson.M{"_id": groupID}).Decode(&g)
			if err != nil {
				_, err = ctx.EffectiveMessage.Reply(b, "هیج کاربری ثبت نشده است", nil)
				if err != nil {
					return fmt.Errorf("failed to send message: %w", err)
				}
				return nil
			}
			cursor, err = peersCollection.Find(context.TODO(), bson.M{"groupID": groupID})
			if err != nil {
				_, err := ctx.EffectiveMessage.Reply(b, "درخواست با خطا مواجه شد", nil)
				if err != nil {
					return fmt.Errorf("failed to send message: %w", err)
				}
				return nil
			}
			if err = cursor.All(context.TODO(), &peers); err != nil {
				_, err := ctx.EffectiveMessage.Reply(b, "درخواست با خطا مواجه شد", nil)
				if err != nil {
					return fmt.Errorf("failed to send message: %w", err)
				}
				return nil
			}
			text += fmt.Sprintf("\n┏ <b>%s</b>\n┃\n", g.Name)
			for _, p := range peers {
				text += fmt.Sprintf("┣━ <i>%s</i>\n", p.Name)
			}
			text += "┃\n"
			text += fmt.Sprintf("┣━━ %s / %s\n┗━━ %s", formatBytes(g.TotalRX+g.TotalTX, true), formatBytes(g.AllowedUsage, true), formatExpiry(g.ExpiresAt, false))
		}
		_, err = ctx.EffectiveMessage.Reply(b, text, &gotgbot.SendMessageOpts{ParseMode: "HTML"})
		if err != nil {
			return fmt.Errorf("failed to send message: %w", err)
		}
		return nil
	}))

	dispatcher.AddHandler(handlers.NewMessage(message.Text, func(b *gotgbot.Bot, ctx *ext.Context) error {
		_, err := ctx.EffectiveMessage.Reply(b, "درخواست نامعتبر", nil)
		if err != nil {
			return fmt.Errorf("failed to send message: %w", err)
		}
		return nil
	}))

	dispatcher.AddHandler(handlers.NewNamedhandler("businesss_message_handler", handlers.Message{
		Filter:        nil,
		AllowBusiness: true,
		Response: func(b *gotgbot.Bot, ctx *ext.Context) error {
			if ctx.BusinessMessage != nil && strings.EqualFold(ctx.BusinessMessage.From.Username, config.AdminID) && strings.HasPrefix(ctx.BusinessMessage.Text, "# ") {
				// name days gigabytes
				parts := strings.Split(ctx.BusinessMessage.Text[2:], " ")
				if len(parts) != 3 {
					return fmt.Errorf("invalid message: %s", ctx.BusinessMessage.Text)
				}
				allowedUsage, err := strconv.ParseInt(parts[1], 10, 64)
				if err != nil {
					return fmt.Errorf("invalid message: %s", ctx.BusinessMessage.Text)
				}
				expiresAt, err := strconv.ParseInt(parts[2], 10, 64)
				if err != nil {
					return fmt.Errorf("invalid message: %s", ctx.BusinessMessage.Text)
				}
				c := http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, Timeout: time.Second * 5}
				req, err := http.NewRequest("POST", "https://10.0.0.1/api/peers", bytes.NewBuffer([]byte(fmt.Sprintf(
					`{"name":"%s","allowedUsage":%d,"expiresAt":%d,"role":"user"}`, parts[0], allowedUsage*1024000000, time.Now().UnixMilli()+(expiresAt*24*3600*1000),
				))))
				if err != nil {
					return fmt.Errorf("failed to create new peer: %w", err)
				}
				req.Header.Set("content-type", "application/json")
				req.Header.Set("bypass_key", config.BypassKey)
				res, err := c.Do(req)
				if err != nil {
					return fmt.Errorf("failed to create new peer: %w", err)
				}
				if res.StatusCode == 201 {
					by, err := io.ReadAll(res.Body)
					if err != nil {
						return fmt.Errorf("failed to create new peer: %w", err)
					}
					req2, err := http.NewRequest("GET", "https://10.0.0.1/api/peers/"+url.QueryEscape(string(by)), nil)
					if err != nil {
						return fmt.Errorf("failed to create new peer: %w", err)
					}
					req2.Header.Set("bypass_key", config.BypassKey)
					res, err = c.Do(req2)
					if err != nil {
						return fmt.Errorf("failed to create new peer: %w", err)
					}
					by, err = io.ReadAll(res.Body)
					if err != nil {
						return fmt.Errorf("failed to create new peer: %w", err)
					}
					var newPeer Peer
					err = json.Unmarshal(by, &newPeer)
					if err != nil {
						return fmt.Errorf("failed to create new peer: %w", err)
					}
					_, err = exec.Command(filepath.Join(path, "croc-qr-generator", "croc-qr-generator"), newPeer.PrivateKey, newPeer.AllowedIPs, config.ServerPublicKey, config.Endpoint, newPeer.Name).CombinedOutput()
					if err != nil {
						return fmt.Errorf("failed to create new peer: %w", err)
					}
					f, err := os.Open(newPeer.Name + ".png")
					if err != nil {
						return fmt.Errorf("failed to open file: %w", err)
					}
					_, err = b.SendPhoto(ctx.BusinessMessage.Chat.Id, gotgbot.InputFileByReader("qr.png", f), &gotgbot.SendPhotoOpts{
						BusinessConnectionId: ctx.BusinessMessage.BusinessConnectionId,
					})
					if err != nil {
						return fmt.Errorf("failed to send photo: %w", err)
					}
				} else if res.StatusCode == 400 {
					_, err = b.SendMessage(ctx.BusinessMessage.Chat.Id, "duplicate name", &gotgbot.SendMessageOpts{
						BusinessConnectionId: ctx.BusinessMessage.BusinessConnectionId,
					})
					if err != nil {
						return fmt.Errorf("failed to send message: %w", err)
					}
				} else {
					return fmt.Errorf("failed to create new peer: %d", res.StatusCode)
				}
			}
			return nil
		},
	}))

	// Start receiving updates.
	err := updater.StartPolling(b, &ext.PollingOpts{
		DropPendingUpdates: true,
		GetUpdatesOpts: &gotgbot.GetUpdatesOpts{
			Timeout: 9,
			RequestOpts: &gotgbot.RequestOpts{
				Timeout: time.Second * 10,
			},
		},
	})
	if err != nil {
		panic("Failed to start polling: " + err.Error())
	}
	log.Printf("%s has been started...\n", b.User.Username)

	for {
		t := time.Now().UnixMilli()
		var peers []*Peer
		cursor, err := peersCollection.Find(context.TODO(), bson.D{})
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
				_, err := b.SendMessage(p.TelegramChatID, fmt.Sprintf("ترافیک قابل استفاده اشتراک <b>%s</b> کمتر از یک گیگابایت است.\n\n<a href=\"https://t.me/%s\">تمدید اشتراک</a>", p.Name, config.AdminID), &gotgbot.SendMessageOpts{ParseMode: "HTML"})
				if err != nil {
					log.Println(err)
					continue
				}
				_, err = peersCollection.UpdateOne(context.TODO(), bson.M{"publicKey": p.PublicKey}, bson.M{"$set": bson.M{"receivedUsageNotice": true}})
				if err != nil {
					log.Println(err)
					continue
				}
			} else if p.ReceivedUsageNotice && p.AllowedUsage-(p.TotalRX+p.TotalTX) > 1024000000 {
				_, err = peersCollection.UpdateOne(context.TODO(), bson.M{"publicKey": p.PublicKey}, bson.M{"$set": bson.M{"receivedUsageNotice": false}})
				if err != nil {
					log.Println(err)
					continue
				}
			} else if !p.ReceivedExpiryNotice && p.ExpiresAt-t < 86400000 {
				_, err := b.SendMessage(p.TelegramChatID, fmt.Sprintf("اشتراک <b>%s</b> کمتر از ۲۴ ساعت دیگر به پایان میرسد.\n\n<a href=\"https://t.me/%s\">تمدید اشتراک</a>", p.Name, config.AdminID), &gotgbot.SendMessageOpts{ParseMode: "HTML"})
				if err != nil {
					log.Println(err)
					continue
				}
				_, err = peersCollection.UpdateOne(context.TODO(), bson.M{"publicKey": p.PublicKey}, bson.M{"$set": bson.M{"receivedExpiryNotice": true}})
				if err != nil {
					log.Println(err)
					continue
				}
			} else if p.ReceivedExpiryNotice && p.ExpiresAt-t > 86400000 {
				_, err = peersCollection.UpdateOne(context.TODO(), bson.M{"publicKey": p.PublicKey}, bson.M{"$set": bson.M{"receivedExpiryNotice": false}})
				if err != nil {
					log.Println(err)
					continue
				}
			}
		}

		time.Sleep(time.Second * 3)
	}
}
