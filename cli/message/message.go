package message

import (
	"context"
	"fmt"

	"csgclaw/cli/command"
	"csgclaw/internal/apitypes"
)

type cmd struct{}

func NewCmd() command.Command {
	return cmd{}
}

func (cmd) Name() string {
	return "message"
}

func (cmd) Summary() string {
	return "Send an IM message."
}

func (c cmd) Run(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("message", run.Program+" message [flags]", "Send an IM message.")
	channelName := fs.String("channel", "csgclaw", "channel name: csgclaw or feishu")
	roomID := fs.String("room-id", "", "target room id")
	senderID := fs.String("sender-id", "", "sender user id")
	content := fs.String("content", "", "message content")
	mentionID := fs.String("mention-id", "", "mentioned user id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("message does not accept positional arguments")
	}
	if *roomID == "" {
		return fmt.Errorf("room_id is required")
	}
	if *senderID == "" {
		return fmt.Errorf("sender_id is required")
	}
	if *content == "" {
		return fmt.Errorf("content is required")
	}

	message, err := run.APIClient(globals).SendMessageByChannel(ctx, *channelName, apitypes.CreateMessageRequest{
		RoomID:    *roomID,
		SenderID:  *senderID,
		Content:   *content,
		MentionID: *mentionID,
	})
	if err != nil {
		return err
	}
	return command.RenderMessages(globals.Output, run.Stdout, []apitypes.Message{message})
}
