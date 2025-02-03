package main

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/dhowden/tag"
	"github.com/gocolly/colly"
	"github.com/jonas747/ogg"

	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2/clientcredentials"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

const (
	servicesOn = false
)

var (
	ctx           context.Context  = nil
	ytService     *youtube.Service = nil
	spotifyClient *spotify.Client  = nil
	isPlaying                      = false
	repeat                         = false
	doInterrupt                    = false
	charCount                      = 0
	isPaused                       = false
	queue                          = []string{}
	index                          = 0
	count                          = 0

	commands = []*discordgo.ApplicationCommand{
		{
			Name:        "play",
			Description: "join the voice channel of the messege author and play the song on path",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:         discordgo.ApplicationCommandOptionString,
					Name:         "search",
					Description:  "query for songs",
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
			Name:        "next",
			Description: "jump to the next song",
		},
		{
			Name:        "prev",
			Description: "jump to the previous song",
		},
		{
			Name:        "pause",
			Description: "pause the music",
		},
		{
			Name:        "shuffle",
			Description: "shuffle current queue",
		},
		{
			Name:        "repeat",
			Description: "toggle repeat option",
		},
		{
			Name:        "queue",
			Description: "display current queue",
		},
		{
			Name:        "queue-all",
			Description: "display all history of the current queue",
		},
		{
			Name:        "clear",
			Description: "clear all history of the current queue",
		},
		{
			Name:        "save",
			Description: "save the current history to a playlist",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:         "name",
					Description:  "name of the playlist",
					Type:         discordgo.ApplicationCommandOptionString,
					Required:     true,
					Autocomplete: true,
				},
			},
		},
		{
			Name:        "basic-command",
			Description: "Basic command",
		},
	}
	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"play": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			switch i.Type {
			case discordgo.InteractionApplicationCommandAutocomplete:
				charCount += 1
				myCharCount := charCount

				filter := i.ApplicationCommandData().Options[0].StringValue()
				choices := []*discordgo.ApplicationCommandOptionChoice{}

				link, err := url.Parse(filter)
				if strings.Contains(link.Hostname(), "spotify") || strings.Contains(link.Hostname(), "youtu") {
					return
				}
				pl, err := os.ReadDir("Playlist")
				if err != nil {
					panic(err)
				}
				for _, file := range pl {
					if strings.Contains(strings.ToLower(file.Name()), strings.ToLower(filter)) {
						choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
							Name:  "📀" + file.Name(),
							Value: "pl:" + file.Name(),
						})
					}
				}

				dir, err := os.ReadDir("Music")
				if err != nil {
					panic(err)
				}
				for _, file := range dir {
					var sb strings.Builder

					f, err := os.Open("Music/" + file.Name())
					if err != nil {
						panic(err)
					}
					defer f.Close()

					metadata, err := tag.ReadFrom(f)
					if err != nil {
					} else {
						if !strings.Contains(file.Name(), metadata.Artist()) {
							sb.WriteString("👥")
							sb.WriteString(metadata.Artist())
							sb.WriteString(" | ")
						}
					}

					sb.WriteString(file.Name())

					if strings.Contains(strings.ToLower(sb.String()), strings.ToLower(filter)) {
						choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
							Name:  "🎵" + sb.String(),
							Value: "local:" + file.Name(),
						})
						if len(choices) > 24 {
							break
						}
					}
				}

				//25 is the discord choice limit
				maxResults := 24 - len(choices)
				if maxResults < 0 {
					charCount += 1
				}
				time.Sleep(2 * time.Second)

				if myCharCount < charCount || len(filter) < 11 || maxResults < 10 {
					s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
						Type: discordgo.InteractionApplicationCommandAutocompleteResult,
						Data: &discordgo.InteractionResponseData{
							Choices: choices,
						},
					})
					return
				}

				fmt.Println("searching using yhoutube api :(", filter)

				call := ytService.Search.List([]string{"id", "snippet"}).
					Q(filter).
					Type("video").
					VideoCategoryId("10").
					MaxResults(10)

				response, err := call.Do()
				if err != nil {
					fmt.Println("fudeu playboy", err)
				}

				for _, item := range response.Items {
					switch item.Id.Kind {
					case "youtube#video":
						choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
							Name:  "⬇️" + item.Snippet.Title,
							Value: "video:" + item.Id.VideoId,
						})

					case "youtube#channel":
						//fodasse
					case "youtube#playlist":
						choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
							Name:  "📀" + item.Snippet.Title,
							Value: "playlist:" + item.Id.PlaylistId,
						})
					}
				}

				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionApplicationCommandAutocompleteResult,
					Data: &discordgo.InteractionResponseData{
						Choices: choices,
					},
				})
			case discordgo.InteractionApplicationCommand:
				charCount = 0
				isLink := false
				guild, err := s.State.Guild(i.GuildID)
				if err != nil {
					fmt.Println("Cound`t fing guild id:", err)
					return
				}

				options := i.ApplicationCommandData().Options
				optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(options))
				for _, opt := range options {
					optionMap[opt.Name] = opt
				}

				println("choice: ", options[0].StringValue())

				link, err := url.Parse(options[0].StringValue())
				if strings.Contains(link.Hostname(), "spotify") {
					isLink = true
					fmt.Println("Chegou no negocio certo")
					split := strings.Split(link.Path, "/")
					if split[1] == "playlist" {
						fmt.Println("Split 2 eh igual a : ", split[2])
						links := parseSpotifyPlaylist(spotify.ID(split[2]))

						s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
							Type: discordgo.InteractionResponseChannelMessageWithSource,
							Data: &discordgo.InteractionResponseData{
								Content: "added the [playlist](" + link.String() + ") to the queue",
							},
						})

						for _, link := range links {
							download(link)
							if isLink && !isPlaying {
								for _, vs := range guild.VoiceStates {
									if vs.UserID == i.Member.User.ID {
										fmt.Println("esta dando join")
										fmt.Println("queue size: ", len(queue))

										go join(guild.ID, vs.ChannelID, s)
									}
								}
							}
						}
					}

					return
				}
				if strings.Contains(link.Hostname(), "youtu") {
					isLink = true
					s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
						Type: discordgo.InteractionResponseChannelMessageWithSource,
						Data: &discordgo.InteractionResponseData{
							Content: "adding the [song](" + options[0].StringValue() + ")...",
						},
					})
					download(options[0].StringValue())
				}

				if isLink && !isPlaying {
					for _, vs := range guild.VoiceStates {
						if vs.UserID == i.Member.User.ID {
							fmt.Println("esta dando join")
							fmt.Println("queue size: ", len(queue))

							join(guild.ID, vs.ChannelID, s)
						}
					}
					return
				}

				if option, ok := optionMap["search"]; ok {
					split := strings.Split(option.StringValue(), ":")

					if len(split) < 2 {
						dir, err := os.ReadDir("Music")
						if err != nil {
							// TODO:
						}

						filter := i.ApplicationCommandData().Options[0].StringValue()
						for _, file := range dir {
							if strings.Contains(strings.ToLower(file.Name()), strings.ToLower(filter)) {
								queue = append(queue, file.Name())
								s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
									Type: discordgo.InteractionResponseChannelMessageWithSource,
									Data: &discordgo.InteractionResponseData{
										Content: "> the song: ***" + file.Name() + "*** was added with success",
									},
								})
								if !isPlaying {
									for _, vs := range guild.VoiceStates {
										if vs.UserID == i.Member.User.ID {
											fmt.Println("esta dando join")
											fmt.Println("queue size: ", len(queue))

											join(guild.ID, vs.ChannelID, s)
										}
									}
									return
								}
								return
							}
						}
						s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
							Type: discordgo.InteractionResponseChannelMessageWithSource,
							Data: &discordgo.InteractionResponseData{
								Content: "> no music local found",
							},
						})
						return
					}

					mode := split[0]
					path := split[1]

					if mode == "video" {
						s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
							Type: discordgo.InteractionResponseChannelMessageWithSource,
							Data: &discordgo.InteractionResponseData{
								Content: "> adding song if the [link](https://www.youtube.com/watch?v=" + path + ")",
							},
						})

						download("https://www.youtube.com/watch?v=" + path)
						time.Sleep(1 * time.Second)
					}
					if mode == "local" {
						queue = append(queue, path)
						s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
							Type: discordgo.InteractionResponseChannelMessageWithSource,
							Data: &discordgo.InteractionResponseData{
								Content: "> the song: ***" + path + "*** was added with success",
							},
						})
					}
					if mode == "pl" {
						pl, err := os.Open("Playlist/" + path)
						if err != nil {
							panic(err)
						}
						file, err := io.ReadAll(pl)
						if err != nil {
							fmt.Println("could not read playlist file")
						}
						list := string(file)
						split := strings.Split(list, "\n")

						for _, path := range split {
							queue = append(queue, path)
						}
						s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
							Type: discordgo.InteractionResponseChannelMessageWithSource,
							Data: &discordgo.InteractionResponseData{
								Content: "> the playlist: ***" + path + "*** was added with success",
							},
						})
					}
				}

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

			fileList.WriteString(">>>")

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
		"next": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "next song...",
				},
			})
			doInterrupt = true
		},
		"prev": func(s *discordgo.Session, i *discordgo.InteractionCreate) {

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "previous song...",
				},
			})
			if index > 0 {
				index -= 2
			}
			doInterrupt = true
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
		"shuffle": func(s *discordgo.Session, i *discordgo.InteractionCreate) {

			for i := range queue[index:] {
				j := rand.Intn(i + 1)
				queue[i], queue[j] = queue[j], queue[i]
			}

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "current queue shuffled",
				},
			})
		},
		"repeat": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			repeat = !repeat

			repeatString := ""

			if repeat {
				repeatString = "true"
			} else {
				repeatString = "false"
			}

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "the repeat option is now set to" + repeatString,
				},
			})
		},
		"queue": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			var contentText strings.Builder
			contentText.WriteString(">>> # Queue\n")
			for _, songs := range queue[index:] {
				contentText.WriteString("* " + songs + "\n")
			}

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: contentText.String(),
				},
			})
		},
		"queue-all": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			var contentText strings.Builder
			contentText.WriteString(">>> # Queue\n")
			for _, songs := range queue {
				contentText.WriteString("* " + songs + "\n")
			}

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: contentText.String(),
				},
			})
		},
		"clear": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			queue = []string{}
			index = 0

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "the queue was cleared",
				},
			})
		},
		"save": func(s *discordgo.Session, i *discordgo.InteractionCreate) {

			switch i.Type {
			case discordgo.InteractionApplicationCommandAutocomplete:

				filter := i.ApplicationCommandData().Options[0].StringValue()
				choices := []*discordgo.ApplicationCommandOptionChoice{}

				dir, err := os.ReadDir("Playlist")
				if err != nil {
					panic(err)
				}
				for _, file := range dir {
					if strings.Contains(strings.ToLower(file.Name()), strings.ToLower(filter)) {
						choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
							Name:  file.Name(),
							Value: file.Name(),
						})
					}
				}

			case discordgo.InteractionApplicationCommand:
				filter := i.ApplicationCommandData().Options[0].StringValue()

				playlist, err := os.Create("./Playlist/" + filter)
				if err != nil {
					fmt.Println("could not open file")
				}
				defer playlist.Close()

				for _, path := range queue {
					_, err := playlist.WriteString(path + "\n")
					if err != nil {
						fmt.Println("erro na playlist")
					}
				}

				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "created the playlist " + filter,
					},
				})
			}

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
	context := context.Background()
	ctx = context

	newService, err := youtube.NewService(ctx, option.WithAPIKey(os.Getenv("YOUTUBE_API_KEY")))
	if err != nil {
		fmt.Println("Error creating new YouTube client: %v", err)
	}
	ytService = newService

	config := &clientcredentials.Config{
		ClientID:     os.Getenv("SPOTIFY_CLIENT_ID"),
		ClientSecret: os.Getenv("SPOTIFY_CLIENT_SECRET"),
		TokenURL:     spotifyauth.TokenURL,
	}
	token, err := config.Token(ctx)
	if err != nil {
		fmt.Println("couldn't get token: %v", err)
	}

	httpClient := spotifyauth.New().Client(ctx, token)
	spotifyClient = spotify.New(httpClient)

	dg, err := discordgo.New("Bot " + os.Getenv("DISCORD_TOKEN"))
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
		if len(queue) == 0 {
			continue
		}
		if index+1 > len(queue) {
			break
		}

		isPlaying = true
		file, err := os.Open("Music/" + queue[index])
		fmt.Println("Current music playing: ", queue[index])
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
			if doInterrupt {
				doInterrupt = false
				break
			}
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
		if index-1 >= len(queue) {
			index = 0
		}
	}
}

func parseSpotifyPlaylist(id spotify.ID) []string {
	links := []string{}

	fmt.Println("parsing the playlist")

	c := colly.NewCollector()

	fields := spotify.Fields("items(track)")

	results, err := spotifyClient.GetPlaylistItems(ctx, id, fields)
	if err != nil {
		fmt.Println("could not find the playlist")
	}
	fmt.Println("got the results")
	fmt.Println("Lenght", len(results.Items))

	for _, item := range results.Items {

		linkFound := false

		filter := item.Track.Track.Name + " " + item.Track.Track.Artists[0].Name

		fmt.Println("This is the filter: ", filter)

		c.OnHTML("a[href]", func(e *colly.HTMLElement) {
			if linkFound {
				return
			}
			if e.Attr("class") == "result__url" {
				linkFound = true
				links = append(links, e.Text)
			}
		})

		var url strings.Builder
		url.WriteString("https://html.duckduckgo.com/html/?q=youtube%20")

		split := strings.Split(filter, " ")

		for _, s := range split {
			url.WriteString("%20")
			url.WriteString(strings.Trim(s, "&"))
		}

		err := c.Visit(url.String())
		if err != nil {
			println(err)
		}
	}

	return links

}

func download(link string) {
	stdout, err := exec.Command("./yt-dlp", "--cookies", "./Secret/www.youtube.com_cookies.txt", "-x", "--embed-metadata", "-o", "./Music/%(title)s", "--audio-format", "mp3", link).Output()
	if err != nil {
		fmt.Println("could not download video")
	}

	fmt.Println(string(stdout))

	splitTest := strings.Split(string(stdout), "\"")
	for i, s := range splitTest {
		if i%2 == 1 {
			queue = append(queue, s[8:])
		}
	}

}
