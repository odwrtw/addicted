Addicted
=====

[![GoDoc](https://godoc.org/github.com/odwrtw/addicted?status.svg)](http://godoc.org/github.com/odwrtw/addicted)
[![Go Report Card](https://goreportcard.com/badge/github.com/odwrtw/addicted)](https://goreportcard.com/report/github.com/odwrtw/addicted)


Usage
=====

    package main

    import (
            "context"
            "io"
            "os"

            "github.com/odwrtw/addicted"
    )

    func main() {
            ctx := context.Background()

            // Create a client
            client, err := addicted.NewWithAuth("username", "yourpassword")
            if err != nil {
                panic(err)
            }

            // Get subtitles for a show by name (House of Cards S01E01)
            subs, err := client.GetSubtitles(ctx, "House of Cards", 1, 1)
            if err != nil {
                panic(err)
            }

            // Filter by language
            subs = subs.FilterByLang("french")
            if len(subs) == 0 {
                panic("no subtitles found")
            }

            // Download the subtitle content
            rc, err := client.Download(ctx, subs[0])
            if err != nil {
                panic(err)
            }
            defer rc.Close()

            f, err := os.Create("subtitle.srt")
            if err != nil {
                panic(err)
            }
            defer f.Close()

            io.Copy(f, rc)
    }
