package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/ogg"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

var (
	ytService             *youtube.Service = nil
	isPlaying                              = false
	emptyqueue                             = true
	isPaused                               = false
	integerOptionMinValue                  = 1.0
	dmPermission                           = false
	queue                                  = []string{""}
	index                                  = 0

	commands = []*discordgo.ApplicationCommand{
		{
			Name:        "ytdl",
			Description: "Download the music file from a youtube link",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "link",
					Description: "the youtube link that is gonna be downloaded",
					Required:    true,
				},
			},
		},
		{
			Name:        "play",
			Description: "join the voice channel of the messege author and play the song on path",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:         discordgo.ApplicationCommandOptionString,
					Name:         "path",
					Description:  "the path to the music",
					Required:     true,
					Autocomplete: true,
				},
			},
		},
		{
			Name:        "list",
			Description: "list local files",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "search",
					Description: "Filter for the local files",
				},
			},
		},

		{
			Name:        "pause",
			Description: "toggle the music",
		},
		{
			Name:        "queue",
			Description: "display current queue",
		},
		{
			Name:        "basic-command",
			Description: "Basic command",
		},
	}
	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"ytdl": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			options := i.ApplicationCommandData().Options

			optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(options))
			for _, opt := range options {
				optionMap[opt.Name] = opt
			}

			if option, ok := optionMap["link"]; ok {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "Hey there! Congratulations, you just dowloaded a youtube video",
					},
				})

				// TODO: error messege if invalid link
				download(option.StringValue())
				return
			}

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					// TODO: better error messege
					Content: "Something went wrong :(",
				},
			})
		},
		"play": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			switch i.Type {
			case discordgo.InteractionApplicationCommandAutocomplete:
				filter := i.ApplicationCommandData().Options[0].StringValue()
				choices := []*discordgo.ApplicationCommandOptionChoice{}

				dir, err := os.ReadDir("Music")
				if err != nil {
					panic(err)
				}

				for _, file := range dir {
					if strings.Contains(strings.ToLower(file.Name()), strings.ToLower(filter)) {
						choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
							Name:  "🎵" + file.Name(),
							Value: "local:" + file.Name(),
						})
					}
				}

				//25 is the discord choice limit
				maxResults := 25 - len(choices)

				if false {
					call := ytService.Search.List([]string{"id", "snippet"}).
						Q(filter).
						VideoCategoryId("Music").
						MaxResults(int64(maxResults))
					response, err := call.Do()
					if err != nil {
						fmt.Println("fudeu playboy", err)
					}

					for _, item := range response.Items {
						switch item.Id.Kind {
						case "youtube#video":
							choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
								Name:  "⬇️" + item.Snippet.Title,
								Value: "video:" + item.Id.VideoId + ":" + item.Snippet.Title,
							})

						case "youtube#channel":
							//fodasse
						case "youtube#playlist":
							choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
								Name:  "📀" + item.Snippet.Title,
								Value: "playlist:" + item.Id.PlaylistId + ":" + item.Snippet.Title,
							})
						}
					}
				}
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionApplicationCommandAutocompleteResult,
					Data: &discordgo.InteractionResponseData{
						Choices: choices,
					},
				})
			case discordgo.InteractionApplicationCommand:
				options := i.ApplicationCommandData().Options
				optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(options))
				for _, opt := range options {
					optionMap[opt.Name] = opt
				}
				if option, ok := optionMap["path"]; ok {
					split := strings.Split(option.StringValue(), ":")

					mode := split[0]
					path := split[1]

					if mode == "video" {
						choiceName := split[2]
						download("https://www.youtube.com/watch?v=" + path)
						queue = append(queue, choiceName)

						s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
							Type: discordgo.InteractionResponseChannelMessageWithSource,
							Data: &discordgo.InteractionResponseData{
								Content: "the song: " + choiceName + " was added with success",
							},
						})

						time.Sleep(5 * time.Second)
					}
					if mode == "playlist" {
						choiceName := split[2]
						download("https://www.youtube.com/playlist?list=" + path)
						queue = append(queue, choiceName)

						s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
							Type: discordgo.InteractionResponseChannelMessageWithSource,
							Data: &discordgo.InteractionResponseData{
								Content: "the song: " + choiceName + " was added with success",
							},
						})

						time.Sleep(5 * time.Second)
					}
					if mode == "local" {
						queue = append(queue, path)
						s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
							Type: discordgo.InteractionResponseChannelMessageWithSource,
							Data: &discordgo.InteractionResponseData{
								Content: "the song: " + path + " was added with success",
							},
						})
					}
				}
				guild, err := s.State.Guild(i.GuildID)
				if err != nil {
					fmt.Println("Cound`t fing guild id:", err)
					return
				}
				if !isPlaying {
					for _, vs := range guild.VoiceStates {
						if vs.UserID == i.Member.User.ID {
							join(guild.ID, vs.ChannelID, s)
						}
					}
				}
			}
		},
		"list": func(s *discordgo.Session, i *discordgo.InteractionCreate) {

			dir, err := os.ReadDir("Music")
			if err != nil {
				// TODO:
			}

			var fileList strings.Builder

			if len(i.ApplicationCommandData().Options) > 0 {
				filter := i.ApplicationCommandData().Options[0].StringValue()
				for _, file := range dir {
					if strings.Contains(strings.ToLower(file.Name()), strings.ToLower(filter)) {
						fileList.WriteString(file.Name() + "\n")
					}
				}
			} else {
				for _, file := range dir {
					fileList.WriteString(file.Name() + "\n")
				}
			}

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Here is a list of local files:\n" + fileList.String(),
				},
			})
		},
		"pause": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			if isPaused {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "Continuing...",
					},
				})
			} else {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "Pausing...",
					},
				})
			}
			isPaused = !isPaused
		},
		"queue": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			var contentText strings.Builder
			for _, songs := range queue[index:] {
				contentText.WriteString(songs + "\n")
			}

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: contentText.String(),
				},
			})
		},
		"basic-command": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Hey there! Congratulations, you just executed your first slash command",
				},
			})
		},
	}
)

func main() {
	ctx := context.Background()

	newService, err := youtube.NewService(ctx, option.WithAPIKey(os.Getenv("YoutubeApiKey")))
	if err != nil {
		fmt.Println("Error creating new YouTube client: %v", err)
	}
	ytService = newService

	dg, err := discordgo.New("Bot " + os.Getenv("DiscordToken"))
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	dg.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
			h(s, i)
		}
	})

	dg.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsGuildVoiceStates

	err = dg.Open()
	if err != nil {
		fmt.Println("error opening connection,", err)
		return
	}
	defer dg.Close()

	registeredCommands := make([]*discordgo.ApplicationCommand, len(commands))
	for i, v := range commands {
		for _, guild := range dg.State.Guilds {
			cmd, err := dg.ApplicationCommandCreate(dg.State.User.ID, guild.ID, v)
			if err != nil {
				fmt.Println("Cannot create '%v' command: %v", v.Name, err)
			}
			registeredCommands[i] = cmd
		}
	}

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}

func join(GuildID string, ChannelID string, session *discordgo.Session) {

	voice, err := session.ChannelVoiceJoin(GuildID, ChannelID, false, true)
	if err != nil {
		fmt.Println("failed to join voice channel:", err)
		return
	}

	for {
		isPlaying = true
		if index+1 > len(queue) {
			isPlaying = false
			break
		}
		file, err := os.Open("Music\\" + queue[index])
		if err != nil {
			fmt.Errorf("failed to music file: %w", err)
		}
		defer file.Close()

		ffmpeg := exec.Command(
			"ffmpeg",
			"-i", "-",
			"-f", "opus",
			"-frame_duration", "20",
			"-ar", "48000",
			"-ac", "2",
			"-",
		)
		ffmpeg.Stdin = file
		audioPipe, err := ffmpeg.StdoutPipe()
		if err != nil {
			fmt.Errorf("failed to tranform into opus: %w", err)
		}
		if err := ffmpeg.Start(); err != nil {
			fmt.Errorf("failed to start ffmpeg into opus: %w", err)
		}

		pageDecoder := ogg.NewDecoder(audioPipe)
		pageDecoder.Decode()
		pageDecoder.Decode()

		voice.Speaking(true)
		packetDecoder := ogg.NewPacketDecoder(pageDecoder)
		for {
			if isPaused {
				continue
			}
			packet, _, err := packetDecoder.Decode()
			if err != nil {
				fmt.Errorf("failed to decode: %w", err)
			}
			if len(packet) < 1 {
				break
			}
			voice.OpusSend <- packet
		}

		isPlaying = false
		voice.Speaking(false)
		index += 1
	}
}

func download(link string) {
	cmd := exec.Command("yt-dlp", "--cookies", "./Secret/www.youtube.com_cookies.txt", "-x", "--embed-metadata", "-o", "./Music/%(title)s", "--audio-format", "mp3", link)
	stdout, err := cmd.Output()
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	fmt.Println(string(stdout))

}
