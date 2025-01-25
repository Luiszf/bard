package main

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/ogg"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

var (
	isPlaying             = false
	integerOptionMinValue = 1.0
	dmPermission          = false

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
			Name:        "join",
			Description: "join the voice channel of the messege author",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "path",
					Description: "the path to the music",
					Required:    true,
				},
			},
		},
		{
			Name:        "list",
			Description: "list local files",
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
			if isPlaying {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "already playing",
					},
				})
				return
			}
			isPlaying = true

			options := i.ApplicationCommandData().Options
			optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(options))
			for _, opt := range options {
				optionMap[opt.Name] = opt
			}
			path := ""
			if option, ok := optionMap["path"]; ok {
				path = option.StringValue()
			}

			guild, err := s.State.Guild(i.GuildID)
			if err != nil {
				fmt.Println("Cound`t fing guild id:", err)
				return
			}

			for _, vs := range guild.VoiceStates {
				if vs.UserID == i.Member.User.ID {
					join(guild.ID, vs.ChannelID, s, path)
				}
			}

		},
		"list": func(s *discordgo.Session, i *discordgo.InteractionCreate) {

			dir, err := os.ReadDir("Music")
			if err != nil {
				// TODO:
			}

			var fileList strings.Builder

			for _, files := range dir {
				fileList.WriteString(files.Name() + "\n")
			}

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Here is a list of local files:\n" + fileList.String(),
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
		// TODO: make it work for all guilds the bot is present
		cmd, err := dg.ApplicationCommandCreate(dg.State.User.ID, dg.State.Guilds[0].ID, v)
		if err != nil {
			fmt.Println("Cannot create '%v' command: %v", v.Name, err)
		}
		registeredCommands[i] = cmd
	}

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}

func join(GuildID string, ChannelID string, session *discordgo.Session, path string) {

	voice, err := session.ChannelVoiceJoin(GuildID, ChannelID, false, true)
	if err != nil {
		fmt.Println("failed to join voice channel:", err)
		return
	}

	file, err := os.Open("Music\\" + path)
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

	// sinaliza ao Discord que estamos enviando áudio
	voice.Speaking(true)
	packetDecoder := ogg.NewPacketDecoder(pageDecoder)
	for {
		packet, _, err := packetDecoder.Decode()
		if err != nil {
			fmt.Errorf("failed to decode: %w", err)
		}
		voice.OpusSend <- packet
	}

	isPlaying = false
	voice.Speaking(false)
}

func download(link string) {
	cmd := exec.Command("yt-dlp", "-x", "--embed-metadata", "-o", "./Music/%(title)s", "--audio-format", "mp3", link)
	stdout, err := cmd.Output()
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	fmt.Println(string(stdout))

}
