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

            "gitlab.quimbo.fr/nicolas/addicted"
    )

    func main() {
            user := "username"
            passwd := "yourpassword"

            addic, err := addicted.New(user, passwd)

            // t, err := addic.GetTvShows()
            // if err != nil {
            // 	fmt.Println(err)
            // }

            // fmt.Println(t)

            sub, err := addic.GetSubtitles("3103", 1, 1)
            if err != nil {
                    fmt.Println(err)
            }
            for i, s := range sub {
                    if s.Language == "French" {
                            fmt.Println(i)
                    }
            }
            subtitle, _ := ioutil.ReadAll(&sub[7])
            ioutil.WriteFile("test.srt", subtitle, 0644)
    }
