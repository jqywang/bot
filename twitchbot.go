/**
 * twitchbot.go
 *
 * Copyright (c) 2017 Forest Hoffman. All Rights Reserved.
 * License: MIT License (see the included LICENSE file)
 */

package twitchbot

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/textproto"
	"regexp"
	"strings"
	"time"
)

var msgRegex *regexp.Regexp = regexp.MustCompile(`^:(\w+)!\w+@\w+\.tmi\.twitch\.tv (PRIVMSG) #\w+(?: :(.*))?$`)
var cmdRegex *regexp.Regexp = regexp.MustCompile(`^!(\w+)\s?(\w+)?`)

// TODO:
// 1. Connect to a Twitch.tv Chat channel.
// 	a. Pass along necessary information for the connection.
// 	 i.   The IRC (chat) server.
// 	 ii.  The port on the server.
// 	 iii. The channel we want the bot to join.
// 	 iv.  The bot's name.
// 	 v.   A secure key to allow the bot to connect indirectly (not through the website).
// 	 vi.  A maximum speed at which the bot can respond.
// 2. Listen for messages in the chat.
// 3. Do things based on what is happening in the chat.

const PSTFormat = "Jan 2 15:04:05 PST"

func timeStamp() string {
	return TimeStamp(PSTFormat)
}

func TimeStamp(format string) string {
	return time.Now().Format(format)
}

func (bb *BernzBot) Connect() {
	var err error
	fmt.Printf("[%s] Connecting to %s... \n", timeStamp(), bb.Server)

	bb.conn, err = net.Dial("tcp", bb.Server+":"+bb.Port)
	if nil != err {
		fmt.Printf("[%s] Cannot connect to %s, retrying.\n", timeStamp(), bb.Server)
		bb.Connect()
		return
	}
	fmt.Printf("[%s] Connected to %s!\n", timeStamp(), bb.Server)
	bb.startTime = time.Now()
}

func (bb *BernzBot) Disconnect() {
	bb.conn.Close()
	upTime := time.Now().Sub(bb.startTime).Seconds()
	fmt.Printf("[%s] Closed connection from %s! | Live for: %fs\n", timeStamp(), bb.Server, upTime)
}

// Listens for and logs messages from chat. Responds to commands from the channel owner. The bot
// continues until it gets disconnected, told to shutdown, or forcefully shutdown.
func (bb *BernzBot) HandleChat() error {
	fmt.Printf("[%s] Watching #%s...\n", timeStamp(), bb.Channel)

	// reads from connection
	tp := textproto.NewReader(bufio.NewReader(bb.conn))

	// listens for chat messages
	for {
		line, err := tp.ReadLine()
		if nil != err {

			// officially disconnects the bot from the server
			bb.Disconnect()

			return errors.New("bb.Bot.HandleChat: Failed to read line from channel. Disconnected.")
		}

		// logs the response from the IRC server
		fmt.Printf("[%s] %s\n", timeStamp(), line)

		if "PING :tmi.twitch.tv" == line {

			// respond to PING message with a PONG message, to maintain the connection
			bb.conn.Write([]byte("PONG :tmi.twitch.tv\r\n"))
			continue
		} else {
			// handle a PRIVMSG message
			matches := msgRegex.FindStringSubmatch(line)
			if nil != matches {
				userName := matches[1]
				msgType := matches[2]

				switch msgType {
				case "PRIVMSG":
					msg := matches[3]
					fmt.Printf("[%s] %s: %s\n", timeStamp(), userName, msg)

					// parse commands from user message
					cmdMatches := cmdRegex.FindStringSubmatch(msg)
					if nil != cmdMatches {
						cmd := strings.ToLower(cmdMatches[1])
						arg := strings.ToLower(cmdMatches[2])
						fmt.Println("Command received: ", cmd, arg)

						// channel-owner specific commands
						if userName == bb.Channel {
							switch cmd {
								case "tbdown":
									fmt.Printf(
										"[%s] Shutdown command received. Shutting down now...\n",
										timeStamp(),
									)

									bb.Disconnect()
									return nil
								case "canyouhearme":
									fmt.Println("hearing you")
									// bb.Counter = bb.Counter + 1
									bb.Say(fmt.Sprintf("hey there, bot reporting in! Couter is something"))
									return nil
								default:
									// do nothing
							}
						}
					}
				default:
					// do nothing
				}
			}
		}
		time.Sleep(bb.MsgRate)
	}
}

// Makes the bot join its pre-specified channel.
func (bb *BernzBot) JoinChannel() {
	fmt.Printf("[%s] Joining #%s...\n", timeStamp(), bb.Channel)
	bb.conn.Write([]byte("PASS " + bb.Credentials.Password + "\r\n"))
	bb.conn.Write([]byte("NICK " + bb.Name + "\r\n"))
	bb.conn.Write([]byte("JOIN #" + bb.Channel + "\r\n"))

	fmt.Printf("[%s] Joined #%s as @%s!\n", timeStamp(), bb.Channel, bb.Name)
}

// Reads from the private credentials file and stores the data in the bot's Credentials field.
func (bb *BernzBot) ReadCredentials() error {

	// reads from the file
	credFile, err := ioutil.ReadFile(bb.PrivatePath)
	if nil != err {
		return err
	}

	bb.Credentials = &OAuthCred{}

	// parses the file contents
	dec := json.NewDecoder(strings.NewReader(string(credFile)))
	if err = dec.Decode(bb.Credentials); nil != err && io.EOF != err {
		return err
	}

	return nil
}

// Makes the bot send a message to the chat channel.
func (bb *BernzBot) Say(msg string) error {
	if "" == msg {
		return errors.New("BernzBot.Say: msg was empty.")
	}
	_, err := bb.conn.Write([]byte(fmt.Sprintf("PRIVMSG #%s %s\r\n", bb.Channel, msg)))
	if nil != err {
		return err
	}
	return nil
}

// Starts a loop where the bot will attempt to connect to the Twitch IRC server, then connect to the
// pre-specified channel, and then handle the chat. It will attempt to reconnect until it is told to
// shut down, or is forcefully shutdown.
func (bb *BernzBot) Start() {
	err := bb.ReadCredentials()
	if nil != err {
		fmt.Println(err)
		fmt.Println("Aborting...")
		return
	}

	for {
		bb.Connect()
		bb.JoinChannel()
		err = bb.HandleChat()
		if nil != err {

			// attempts to reconnect upon unexpected chat error
			time.Sleep(1000 * time.Millisecond)
			fmt.Println(err)
			fmt.Println("Starting bot again...")
		} else {
			return
		}
	}
}

type OAuthCred struct {
	Password string `json:"password,omitempty"`
}

type TwitchBot interface {
	Connect()
	Disconnect()
	HandleChat() error
	JoinChannel()
	ReadCredentials() error
	Say(msg string) error
	Start()
}

type BernzBot struct {

	// The channel that the bot is supposed to join. Note: The name MUST be lowercase, regardless
	// of how the username is displayed on Twitch.tv.
	Channel string

	// The credentials necessary for authentication.
	Credentials *OAuthCred

	conn net.Conn

	// A forced delay between bot responses. This prevents the bot from breaking the message limit
	// rules. A 20/30 millisecond delay is enough for a non-modded bot. If you decrease the delay
	// make sure you're still within the limit!
	//
	// Message Rate Guidelines: https://dev.twitch.tv/docs/irc#irc-command-and-message-limits
	MsgRate time.Duration

	// The name that the bot will use in the chat that it's attempting to join.
	Name string

	// The port of the IRC server.
	Port string

	// A path to a limited-access directory containing the bot's OAuth credentials.
	PrivatePath string

	// The domain of the IRC server.
	Server string

	Counter int

	startTime time.Time
}
