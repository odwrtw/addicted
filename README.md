Addicted
=====

[![GoDoc](https://godoc.org/github.com/odwrtw/addicted?status.svg)](http://godoc.org/github.com/odwrtw/addicted)
[![Go Report Card](https://goreportcard.com/badge/github.com/odwrtw/addicted)](https://goreportcard.com/report/github.com/odwrtw/addicted)


Usage
=====

    package main

    import (
            "fmt"
            "io/ioutil"

            "github.com/odwrtw/addicted"
    )

    func main() {
            user := "username"
            passwd := "yourpassword"

            // Create a client
            addic, err := addicted.NewWithAuth(user, passwd)
            // Or
            // addic, err := addicted.New()

            // Get the list of shows ( addicted has its own system of IDs )
            // shows, err := addic.GetTvShows()
            // if err != nil {
            //     fmt.Println(err)
            // }

            // Get the susbtitles ( House of cards S01E01 )
            subs, err := addic.GetSubtitles("3103", 1, 1)
            if err != nil {
                fmt.Println(err)
            }

            // Filter the results on the lang
            subs = subs.FilterByLang("french")

            // Download it
            subtitle, err := ioutil.ReadAll(&subs[0])
            if err != nil {
                panic(err)
            }
            ioutil.WriteFile("test.srt", subtitle, 0644)
    }
