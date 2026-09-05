package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"sdio/internal/update"
)

// torrentTest manually exercises internal/update's selective
// per-file torrent download against a real .torrent file ("sdigo
// torrenttest ..."), for diagnosing torrent connectivity/webseed
// issues outside of a full scan/install run.
func torrentTest(args []string) int {
	fs := flag.NewFlagSet("torrenttest", flag.ContinueOnError)
	torrentFile := fs.String("torrent", "", "path to a .torrent metadata file (required)")
	dataDir := fs.String("data-dir", "", "directory to download into (required)")
	files := fs.String("files", "", "comma-separated file paths (as shown by -list) to download")
	list := fs.Bool("list", false, "list every file in the torrent and exit")
	timeout := fs.Duration("timeout", 2*time.Minute, "how long to wait for the selected files to complete")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *torrentFile == "" || *dataDir == "" {
		fmt.Fprintln(os.Stderr, "usage: sdigo torrenttest -torrent=<path> -data-dir=<dir> [-list] [-files=a,b,c]")
		fs.PrintDefaults()
		return 2
	}

	if err := torrenttestRun(*torrentFile, *dataDir, *files, *list, *timeout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

func torrenttestRun(torrentFile, dataDir, files string, list bool, timeout time.Duration) error {
	c, err := update.NewClient(update.Config{DataDir: dataDir})
	if err != nil {
		return err
	}
	defer c.Close()

	t, err := c.AddFromFile(torrentFile)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := t.WaitInfo(ctx); err != nil {
		return fmt.Errorf("waiting for torrent metadata: %w", err)
	}
	fmt.Println("torrent name:", t.Name())

	if list {
		for _, f := range t.Files() {
			fmt.Printf("%d\t%s\n", f.Length, f.Path)
		}
		return nil
	}

	if files == "" {
		return fmt.Errorf("no -files given (use -list to see available paths)")
	}
	selected := t.SelectFiles(strings.Split(files, ","))
	if len(selected) == 0 {
		return fmt.Errorf("none of the requested files were found in the torrent")
	}
	for _, f := range selected {
		fmt.Printf("selected %s (%d bytes)\n", f.Path, f.Length)
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		p := t.Progress(selected)
		fmt.Printf("progress: %d/%d (%d%%)\n", p.Completed, p.Total, p.Percent())
		if p.Percent() >= 100 {
			fmt.Println("done")
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timed out after %s", timeout)
}
