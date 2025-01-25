package main

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/ogg"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

func main() {
	dg, err := discordgo.New("Bot " + "your token")
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	defer dg.Close()

	dg.AddHandler(parseMessage)

	dg.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsGuildVoiceStates

	err = dg.Open()
	if err != nil {
		fmt.Println("error opening connection,", err)
		return
	}

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}

func parseMessage(session *discordgo.Session, message *discordgo.MessageCreate) {

	if message.Author.ID == session.State.User.ID {
		return
	}

	fmt.Println("Bot is now running.  Press CTRL-C to exit.")

	channel, err := session.State.Channel(message.ChannelID)
	if err != nil {
		fmt.Println("Cound`t fing channel id:", err)
		return
	}

	guild, err := session.State.Guild(channel.GuildID)
	if err != nil {
		fmt.Println("Cound`t fing guild id:", err)
		return
	}

	for _, vs := range guild.VoiceStates {
		if vs.UserID == message.Author.ID {
			join(message.Content, guild.ID, vs.ChannelID, session)
		}
	}

	fmt.Println(guild.Name)
	fmt.Println("END")
}

func join(link string, GuildID string, ChannelID string, session *discordgo.Session) {

	voice, err := session.ChannelVoiceJoin(GuildID, ChannelID, false, true)
	if err != nil {
		fmt.Println("failed to join voice channel:", err)
		return
	}

	dlp := exec.Command(
		"yt-dlp",
		"--extract-audio", link,
		"-o", "-")
	dlpPipe, err := dlp.StdoutPipe()
	if err != nil {
		fmt.Errorf("failed to get yt-dlp pipe: %w", err)
	}
	dlp.Stderr = os.Stderr // obter saída informativa do yt-dlp

	ffmpeg := exec.Command(
		"ffmpeg",
		"-i", "-",
		"-f", "opus",
		"-frame_duration", "20",
		"-ar", "48000",
		"-ac", "2",
		"-",
	)
	ffmpegPipe, err := ffmpeg.StdoutPipe()
	if err != nil {
		fmt.Errorf("failed to get ffmpeg pipe: %w", err)
	}
	ffmpeg.Stdin = dlpPipe // associa saída do yt-dlp à entrada do ffmpeg
	ffmpeg.Stderr = os.Stderr

	if err := dlp.Start(); err != nil {
		fmt.Errorf("failed to start yt-dlp: %w", err)
	}
	if err := ffmpeg.Start(); err != nil {
		fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	pageDecoder := ogg.NewDecoder(ffmpegPipe)
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

	voice.Speaking(false)
}

func download(link string) {
	cmd := exec.Command("yt-dlp", "-x", "--embed-metadata", "-o", "./Music/%(title)s.", "--audio-format", "mp3", link)
	stdout, err := cmd.Output()
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	fmt.Println(string(stdout))

}
