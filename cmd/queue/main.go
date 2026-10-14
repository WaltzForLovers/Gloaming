package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/WaltzForLovers/Gloaming/internal/apperr"
	"github.com/WaltzForLovers/Gloaming/internal/auth"
	"github.com/WaltzForLovers/Gloaming/internal/catalog"
	"github.com/WaltzForLovers/Gloaming/internal/events"
	"github.com/WaltzForLovers/Gloaming/internal/httpapi"
	"github.com/WaltzForLovers/Gloaming/internal/limit"
	"github.com/WaltzForLovers/Gloaming/internal/mail"
	"github.com/WaltzForLovers/Gloaming/internal/metrics"
	"github.com/WaltzForLovers/Gloaming/internal/posters"
	"github.com/WaltzForLovers/Gloaming/internal/queue"
	"github.com/WaltzForLovers/Gloaming/internal/storage"
	"github.com/WaltzForLovers/Gloaming/web"
)

func main() {
	log.SetFlags(0)
	addr := os.Getenv("SLATE_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	dsn := os.Getenv("GLOAMING_DSN")
	if dsn == "" {
		dsn = "postgres://127.0.0.1/gloaming?sslmode=disable"
	}

	db, err := storage.Open(dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, apperr.From(err).Message)
		log.Printf("%v", err)
		os.Exit(1)
	}
	defer db.Close()

	catalogURL := os.Getenv("SLATE_CATALOG")
	if catalogURL == "" {
		catalogURL = "https://api.tvmaze.com"
	}
	shows := catalog.NewHTTPClient(catalogURL)
	films := catalog.NewWikidata("https://www.wikidata.org", "https://en.wikipedia.org")
	redisURL := os.Getenv("GLOAMING_REDIS")
	if redisURL == "" {
		redisURL = "redis://127.0.0.1:6379/0"
	}
	var gate limit.Gate
	if redisURL != "off" {
		cache, err := catalog.OpenRedis(redisURL)
		if err != nil {
			log.Printf("redis: %v", err)
		} else {
			shows.WithCache(cache)
			films.WithCache(cache)
		}
		if lim, err := limit.Open(redisURL); err != nil {
			log.Printf("redis limit: %v", err)
		} else {
			gate = lim
		}
	}
	publicURL := os.Getenv("GLOAMING_PUBLIC_URL")
	if publicURL == "" {
		publicURL = "http://127.0.0.1:8080"
	}
	smtpAddr := os.Getenv("GLOAMING_SMTP")
	if smtpAddr == "" {
		smtpAddr = "127.0.0.1:1025"
	}
	from := os.Getenv("GLOAMING_MAIL_FROM")
	if from == "" {
		from = "gloaming@localhost"
	}
	var sender mail.Sender
	if smtpAddr != "off" {
		sender = mail.Open(smtpAddr, from)
	}
	svc := catalog.New(db, shows).WithFilms(films).UseMail(sender)
	posterDir := os.Getenv("GLOAMING_POSTERS")
	if posterDir == "" {
		posterDir = "posters"
	}
	if store, err := posters.Open(posterDir); err != nil {
		log.Printf("posters: %v", err)
	} else {
		svc.UsePosters(store)
	}
	kafkaAddr := os.Getenv("GLOAMING_KAFKA")
	if kafkaAddr == "" {
		kafkaAddr = "127.0.0.1:9092"
	}
	if kafkaAddr == "off" {
		svc.StartAssembler()
	} else if bus, err := events.Open(kafkaAddr); err != nil {
		log.Printf("kafka: %v", err)
		svc.StartAssembler()
	} else {
		svc.UsePublisher(bus)
		if err := bus.Ensure(events.TopicWatched); err != nil {
			log.Printf("kafka topic: %v", err)
		}
		if err := bus.Ensure(events.TopicCard); err != nil {
			log.Printf("kafka topic: %v", err)
		}
		bus.Consume(context.Background(), events.TopicWatched, "gloaming-watched", svc.HandleWatched)
		bus.Consume(context.Background(), events.TopicCard, "gloaming-card", svc.HandleCard)
	}
	accounts := auth.New(db).UseMail(sender, publicURL)
	handler := metrics.Wrap(httpapi.Guard(posters.Mount(posterDir, httpapi.New(accounts, svc, queue.New(db), os.Stderr, web.Files)), gate))
	if err := httpapi.ListenAndServe(addr, handler); err != nil {
		fmt.Fprintln(os.Stderr, apperr.From(err).Message)
		if apperr.From(err).Code == apperr.CodeInternal {
			log.Printf("%v", err)
		}
		os.Exit(1)
	}
}
