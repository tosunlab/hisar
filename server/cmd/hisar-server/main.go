package main

import (
	"flag"
	"log"

	"github.com/gofiber/fiber/v3"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	app := fiber.New(fiber.Config{AppName: "hisar-server"})
	app.Get("/healthz", func(c fiber.Ctx) error {
		return c.SendString("ok\n")
	})

	log.Fatal(app.Listen(*addr, fiber.ListenConfig{DisableStartupMessage: true}))
}
