package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/robfig/cron/v3"
)

var lastCD time.Time
var start = make(chan int)
var quit = make(chan bool)

var seed = rand.NewSource(time.Now().Unix())
var rnd = rand.New(seed)

var (
	dToken       = flag.String("t", "", "discord autentication token")
	rToken       = flag.String("r", "", "rapidapi autentication token")
	cronSpec     = flag.String("c", "0 1 * * *", "cron spec for periodic actions")
	initialChans = flag.String("i", "", "comma separated string of initial channels to report to")
	passwd       = flag.String("p", "", "password for the bot")
	operators    = flag.String("o", "", "comma separated string of operators for the bot")
	reportCron   *cron.Cron
	reportCronID cron.EntryID
	discord      *discordgo.Session
	// channels for covid announcements
	covChans map[string]struct{}
	// bot operators
	botOps map[string]struct{}
	sc     chan os.Signal
)

func getEnv() {
	*dToken = os.Getenv("DISCORDTOKEN")
	*rToken = os.Getenv("RAPIDAPITOKEN")
	*cronSpec = os.Getenv("TBCRONSPEC")
	*initialChans = os.Getenv("TBCHANS")
	*operators = os.Getenv("TBOPS")
}

func main() {

	getEnv()
	flag.Parse() // flags override env good/bad?

	if *dToken == "" {
		fmt.Println("Usage: dist_twit -t <auth_token>")
		return
	}

	covChans = make(map[string]struct{})
	for _, c := range strings.Split(*initialChans, ",") {
		covChans[c] = struct{}{}
	}
	botOps = make(map[string]struct{})
	for _, c := range strings.Split(*operators, ",") {
		botOps[c] = struct{}{}
	}

	var err error
	discord, err = discordgo.New("Bot " + *dToken)

	if err != nil {
		fmt.Println("error creating Discord session,", err)
		return
	}

	// Register the messageCreate func as a callback for MessageCreate events.
	discord.AddHandler(messageCreate)
	// Open a websocket connection to Discord and begin listening.
	err = discord.Open()
	if err != nil {
		fmt.Println("error opening connection,", err)
		return
	}

	reportCron = cron.New()
	reportCronID, err = reportCron.AddFunc(*cronSpec, cronReport)
	if err == nil {
		reportCron.Start()
	} else {
		fmt.Println(err)
	}

	fmt.Printf("Cronspec is %s\n", *cronSpec)
	// Wait here until CTRL-C or other term signal is received.
	fmt.Println("Bot is now running.  Press CTRL-C to exit.")
	sc = make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Stop cron jobs.
	reportCron.Stop()
	// Cleanly close down the Discord session.
	discord.Close()
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {

	// Ignore all messages created by the bot itself
	// This isn't required in this specific example but it's a good practice.
	if m.Author.ID == s.State.User.ID {
		return
	}
	msg := strings.Split(m.Content, " ")
	switch msg[0] {
	case "!cov": // report covid-19 stats
		if *rToken == "" {
			return
		}
		if time.Now().Sub(lastCD).Seconds() < 10 {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Please wait %.0f seconds and try again.", 10.0-time.Now().Sub(lastCD).Seconds()))
			return
		}
		var err error
		var report string
		lastCD = time.Now()
		if len(msg) > 1 {
			report, err = covid(strings.Join(msg[1:], "-"))
		} else {
			report, err = covid("usa")
		}
		if err == nil {
			s.ChannelMessageSend(m.ChannelID, report)
		}
	case "!reaper": // periodic USA death toll reports
		if len(msg) < 2 || msg[1] != "off" {
			if !isOp(m.Author.ID) {
				return
			}
			if len(msg) == 1 {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Grim Reaper reports are *on* for %s.", chanIDtoMention(m.ChannelID)))
				covChans[m.ChannelID] = struct{}{}
			} else if id, err := chanLinkToID(msg[1]); err == nil {
				covChans[id] = struct{}{}
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Grim Reaper reports are *on* for %s.", chanIDtoMention(id)))
			}
		} else if len(msg) == 2 {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Grim Reaper reports are *off* for %s.", chanIDtoMention(m.ChannelID)))
			delete(covChans, m.ChannelID)
		} else if id, err := chanLinkToID(msg[2]); err == nil {
			delete(covChans, id)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Grim Reaper reports are *off* for %s.", chanIDtoMention(id)))
		}
	case "!op":
		if !isOp(m.Author.ID) {
			return
		}
		count := 0
		users := idstoUsers(msg[1:])
		if len(users) > 0 {
			count += opUsers(users, false)
		}
		count += opUsers(m.Mentions, false)
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("%d user%s added to operators.", count, addS(count)))
	case "!deop":
		if !isOp(m.Author.ID) {
			return
		}
		count := 0
		users := idstoUsers(msg[1:])
		if len(users) > 0 {
			count += opUsers(users, true)
		}
		count += opUsers(m.Mentions, true)
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("%d user%s removed from operators.", count, addS(count)))
	case "!delmsg":
		if len(msg) > 2 {
			s.ChannelMessageDelete(msg[1], msg[2])
		}
	case "!config":
		showConfig(m.Author.ID)
	case "!quit":
		if isOp(m.Author.ID) && m.Message.GuildID == "" {
			sc <- os.Kill
		}
	}
}

func addS(n int) string {
	if n != 1 {
		return "s"
	}
	return ""
}

func idstoUsers(ids []string) []*discordgo.User {
	var users []*discordgo.User
	for _, id := range ids {
		u, err := discord.User(id)
		if err == nil {
			users = append(users, u)
		}
	}
	return users
}
func opUsers(users []*discordgo.User, deop bool) int {
	count := 0
	// users := m.Mentions
	for _, u := range users {
		_, ok := botOps[u.ID]
		if ok && deop {
			delete(botOps, u.ID)
			count++
		} else if !ok && !deop {
			botOps[u.ID] = struct{}{}
			count++
		}
	}
	return count
}

func showConfig(id string) {
	if isOp(id) {
		c, err := discord.UserChannelCreate(id)
		if err == nil {
			discord.ChannelMessageSend(c.ID, fmt.Sprintf("cronspec: %s", *cronSpec))
			time.Sleep(time.Millisecond * 500)
			// discord.ChannelMessageSend(c.ID, "channels:")
			var s = "channels:"
			for k := range covChans {
				s = s + " " + chanIDtoMention(k)
			}
			discord.ChannelMessageSend(c.ID, s)
			time.Sleep(time.Millisecond * 500)
			s = "operators:"
			for k := range botOps {
				s = s + " " + userIDtoMention(k)
			}
			discord.ChannelMessageSend(c.ID, s)
		}
	}
}

func isOp(id string) bool {
	if _, ok := botOps[id]; ok {
		return true
	}
	c, err := discord.UserChannelCreate(id)
	if err == nil {
		discord.ChannelMessageSend(c.ID, "You are not an operator of this bot.")
	}
	return false
}

func userIDtoMention(id string) string {
	u, err := discord.User(id)
	if err == nil {
		return u.Mention()
	}
	return id
}

// Converts a channel ID to a mention. On error it returns the channel ID string.
func chanIDtoMention(id string) string {
	channel, err := discord.State.Channel(id)
	if err == nil {
		return channel.Mention()
	}
	return "channel: " + id
}

// Converts a channel link to an ID. If passed a valid ID it is returned it unchanged.
func chanLinkToID(link string) (id string, err error) {
	id = strings.Replace(strings.Replace(strings.Replace(link, "<", "", 1), ">", "", 1), "#", "", 1)
	_, err = discord.Channel(id)
	if err != nil {
		return "", err
	}
	return id, nil
}
