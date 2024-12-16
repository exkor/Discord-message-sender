package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"io/ioutil"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const configFile = "config.txt"

func getTimestamp() string {
	return fmt.Sprintf("[%s]", time.Now().Format("2006-01-02 15:04:05"))
}

func randomSleep(duration, minRandom, maxRandom int) {
	time.Sleep(time.Duration(duration+rand.Intn(maxRandom-minRandom+1)+minRandom) * time.Second)
}

func readConfig() (map[string]string, int, string, error) {
	config := make(map[string]string)
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, 0, "", fmt.Errorf("%s Config file not found.", getTimestamp())
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		config[key] = value
	}

	if messagesFilePath, exists := config["messages_file"]; !exists {
		return nil, 0, "", fmt.Errorf("%s 'messages_file' key is missing.", getTimestamp())
	} else if delay, err := strconv.Atoi(config["delay_between_messages"]); err != nil {
		return nil, 0, "", fmt.Errorf("%s Invalid delay_between_messages value.", getTimestamp())
	} else {
		return config, delay, messagesFilePath, nil
	}
}

func sendMessage(client *http.Client, channelID, message string, headers map[string]string) {
	data, _ := json.Marshal(map[string]string{"content": message})
	req, err := http.NewRequest("POST", fmt.Sprintf("https://discord.com/api/v9/channels/%s/messages", channelID), bytes.NewBuffer(data))
	if err != nil {
		fmt.Printf("%s Error creating request: %v\n", getTimestamp(), err)
		return
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("%s Error sending message: %v | %s\n", getTimestamp(), err, message)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		fmt.Printf("%s Message sent!\n", getTimestamp())
	}
}

func watchConfigFile(watcher *fsnotify.Watcher) {
	err := watcher.Add(configFile)
	if err != nil {
		fmt.Printf("%s Error adding file to watcher: %v\n", getTimestamp(), err)
		return
	}
	fmt.Printf("%s Watching config file for changes...\n", getTimestamp())
}

func main() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Printf("%s Error creating file watcher: %v\n", getTimestamp(), err)
		return
	}
	defer watcher.Close()

	config, delay, messagesFilePath, err := readConfig()
	if err != nil {
		fmt.Println(err)
		return
	}

	userID, token, channelID, channelURL := config["user_id"], config["token"], config["channel_id"], config["channel_url"]
	headers := map[string]string{
		"Content-Type":  "application/json",
		"User-ID":       userID,
		"Authorization": token,
		"Host":          "discordapp.com",
		"Referrer":      channelURL,
	}
	client := &http.Client{}
	fmt.Printf("%s Messages will be sent to %s with a delay of %d seconds.\n", getTimestamp(), channelURL, delay)

	watchConfigFile(watcher)

	go func() {
		for {
			select {
				case event := <-watcher.Events:
					if event.Op&fsnotify.Write == fsnotify.Write {
						fmt.Printf("%s Config file changed!\n", getTimestamp())
						newConfig, newDelay, newMessagesFilePath, err := readConfig()
						if err == nil {
							config = newConfig
							delay = newDelay
							messagesFilePath = newMessagesFilePath
							fmt.Printf("%s Config reloaded with delay: %d\n", getTimestamp(), delay)
						} else {
							fmt.Println("Error reloading config:", err)
						}
					}
				case err := <-watcher.Errors:
					if err != nil {
						fmt.Printf("%s Watcher error: %v\n", getTimestamp(), err)
					}
			}
		}
	}()

	for {
		messages, err := ioutil.ReadFile(messagesFilePath)
		if err != nil {
			fmt.Printf("%s Messages file not found.\n", getTimestamp())
			return
		}

		for _, message := range strings.Split(string(messages), "\n") {
			if message != "" {
				sendMessage(client, channelID, message, headers)
				randomSleep(delay, 1, 10)
			}
		}

		fmt.Printf("%s Finished sending all messages! Restarting...\n", getTimestamp())
	}
}
