package cli

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"csgclaw/internal/im"
)

func (a *App) runRoom(ctx context.Context, args []string, globals GlobalOptions) error {
	if len(args) == 0 {
		a.usageCommandGroup("room", "Manage rooms.", "csgclaw room <subcommand> [flags]", []string{
			"list               List rooms",
			"create             Create a room",
			"delete <id>        Delete a room",
		})
		return flag.ErrHelp
	}
	if isHelpArg(args[0]) {
		a.usageCommandGroup("room", "Manage rooms.", "csgclaw room <subcommand> [flags]", []string{
			"list               List rooms",
			"create             Create a room",
			"delete <id>        Delete a room",
		})
		return flag.ErrHelp
	}

	switch args[0] {
	case "list":
		return a.runRoomList(ctx, args[1:], globals)
	case "create":
		return a.runRoomCreate(ctx, args[1:], globals)
	case "delete":
		return a.runRoomDelete(ctx, args[1:], globals)
	default:
		a.usageCommandGroup("room", "Manage rooms.", "csgclaw room <subcommand> [flags]", []string{
			"list               List rooms",
			"create             Create a room",
			"delete <id>        Delete a room",
		})
		return fmt.Errorf("unknown room subcommand %q", args[0])
	}
}

func (a *App) runRoomList(ctx context.Context, args []string, globals GlobalOptions) error {
	fs := a.newCommandFlagSet("room list", "csgclaw room list [flags]", "List rooms.")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("room list does not accept positional arguments")
	}

	client := NewAPIClient(globals.Endpoint, globals.Token, a.httpClient)
	rooms, err := client.ListRooms(ctx)
	if err != nil {
		return err
	}
	return a.renderRooms(globals.Output, rooms)
}

func (a *App) runRoomCreate(ctx context.Context, args []string, globals GlobalOptions) error {
	fs := a.newCommandFlagSet("room create", "csgclaw room create [flags]", "Create a room.")
	title := fs.String("title", "", "room title")
	description := fs.String("description", "", "room description")
	creatorID := fs.String("creator-id", "", "room creator id")
	participantIDs := fs.String("participant-ids", "", "comma-separated participant ids")
	locale := fs.String("locale", "", "room locale")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("room create does not accept positional arguments")
	}

	client := NewAPIClient(globals.Endpoint, globals.Token, a.httpClient)
	room, err := client.CreateRoom(ctx, im.CreateRoomRequest{
		Title:          *title,
		Description:    *description,
		CreatorID:      *creatorID,
		ParticipantIDs: parseCSV(*participantIDs),
		Locale:         *locale,
	})
	if err != nil {
		return err
	}
	return a.renderRooms(globals.Output, []im.Room{room})
}

func (a *App) runRoomDelete(ctx context.Context, args []string, globals GlobalOptions) error {
	fs := a.newCommandFlagSet("room delete", "csgclaw room delete <id> [flags]", "Delete a room.")
	if err := fs.Parse(args); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("room delete requires exactly one id")
	}

	client := NewAPIClient(globals.Endpoint, globals.Token, a.httpClient)
	return client.DeleteRoom(ctx, rest[0])
}

func (a *App) renderRooms(output string, rooms []im.Room) error {
	switch output {
	case "", "table":
		return renderRoomsTable(a.stdout, rooms)
	case "json":
		return writeJSON(a.stdout, rooms)
	default:
		return fmt.Errorf("unsupported output format %q", output)
	}
}

func parseCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			items = append(items, part)
		}
	}
	return items
}
