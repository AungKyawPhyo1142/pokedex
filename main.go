package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/AungKyawPhyo1142/pokedex/internal/pokecache"
)

const baseURL = "https://pokeapi.co/api/v2/"

type cliCommand struct {
	name        string
	description string
	callback    func(*config, []string) error
}

type config struct {
	nextURL *string
	prevURL *string
	cache   pokecache.Cache
	pokedex map[string]PokemonInfo
}

type LocationAreaResponse struct {
	Count    int     `json:"count"`
	Next     *string `json:"next"`
	Previous *string `json:"previous"`
	Results  []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"results"`
}

type LocationAreaDetails struct {
	PokemonEncounters []struct {
		Pokemon struct {
			Name string `json:"name"`
		} `json:"pokemon"`
	} `json:"pokemon_encounters"`
}

type PokemonInfo struct {
	Name           string `json:"name"`
	ID             int    `json:"id"`
	BaseExperience int    `json:"base_experience"`
}

func (c *config) fetchLocationArea(url string) (LocationAreaResponse, error) {

	if val, ok := c.cache.Get(url); ok {
		var locationAreaResponse LocationAreaResponse
		err := json.Unmarshal(val, &locationAreaResponse)
		if err != nil {
			return LocationAreaResponse{}, err
		}
		return locationAreaResponse, nil
	}

	res, err := http.Get(url)
	if err != nil {
		return LocationAreaResponse{}, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return LocationAreaResponse{}, err
	}
	c.cache.Add(url, body)

	var locationAreaResponse LocationAreaResponse
	err = json.Unmarshal(body, &locationAreaResponse)
	if err != nil {
		return LocationAreaResponse{}, err
	}
	return locationAreaResponse, nil
}

func (c *config) fetchPokemonInfo(pokemonName string) (PokemonInfo, error) {
	fullURL := baseURL + "/pokemon/" + pokemonName

	if val, ok := c.cache.Get(fullURL); ok {
		var pokemonInfo PokemonInfo
		err := json.Unmarshal(val, &pokemonInfo)
		if err != nil {
			return PokemonInfo{}, err
		}
		return pokemonInfo, nil
	}

	res, err := http.Get(fullURL)
	if err != nil {
		return PokemonInfo{}, err
	}

	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return PokemonInfo{}, err
	}
	c.cache.Add(fullURL, body)

	var pokemon PokemonInfo
	err = json.Unmarshal(body, &pokemon)
	if err != nil {
		return PokemonInfo{}, err
	}

	return pokemon, nil

}

func cleanInput(text string) []string {
	output := strings.ToLower(text)
	words := strings.Fields(output)
	return words
}

func commandExit(c *config, args []string) error {
	fmt.Println("Closing the Pokedex... Goodbye!")
	os.Exit(0)
	return nil
}

func commandHelp(c *config, args []string) error {
	text := `
Welcome to the Pokedex!
Usage:

help: Displays a help message
map: Displays the next 20 location areas
mapb: Displays the previous 20 location areas
explore <location_area>: Lists the pokemon in a given location area
exit: Exit the Pokedex
	`
	fmt.Println(text)
	return nil
}

func commandMap(c *config, args []string) error {
	fullURL := baseURL + "location-area/"
	if c.nextURL != nil {
		fullURL = *c.nextURL
	}
	data, err := c.fetchLocationArea(fullURL)
	if err != nil {
		return err
	}
	c.nextURL = data.Next
	c.prevURL = data.Previous

	for _, loc := range data.Results {
		fmt.Println(loc.Name)
	}
	return nil

}

func commandMapb(c *config, args []string) error {

	if c.prevURL == nil {
		return fmt.Errorf("You are already at the first page")
	}

	url := *c.prevURL
	data, err := c.fetchLocationArea(url)
	if err != nil {
		return err
	}
	c.nextURL = data.Next
	c.prevURL = data.Previous

	for _, loc := range data.Results {
		fmt.Println(loc.Name)
	}
	return nil

}

func commandExplore(c *config, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("you must provide a location area name")
	}
	locationAreaName := args[0]
	url := baseURL + "location-area/" + locationAreaName

	body, ok := c.cache.Get(url)
	if !ok {
		res, err := http.Get(url)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		if res.StatusCode > 299 {
			return fmt.Errorf("bad response from server: %s", res.Status)
		}
		body, err = io.ReadAll(res.Body)
		if err != nil {
			return err
		}
		c.cache.Add(url, body)
	}

	var locationDetails LocationAreaDetails
	err := json.Unmarshal(body, &locationDetails)
	if err != nil {
		return err
	}

	fmt.Printf("Exploring %s...\n", locationAreaName)
	fmt.Println("Found Pokémon:")
	for _, encounter := range locationDetails.PokemonEncounters {
		fmt.Printf(" - %s\n", encounter.Pokemon.Name)
	}
	return nil
}

func commandCatch(c *config, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("you must provide a pokemon name")
	}

	pokemonName := args[0]

	data, err := c.fetchPokemonInfo(pokemonName)
	if err != nil {
		return fmt.Errorf("error fetching pokemon info: %v", err)
	}
	fmt.Printf("Throwing a Pokeball at %s...\n", pokemonName)

	maxExp := 600.0     // around Blissey's base exp
	minCatchRate := 0.1 // 10% minimum chance
	maxCatchRate := 0.9 // 90% maximum chance

	expRatio := float64(data.BaseExperience) / maxExp
	catchRate := maxCatchRate - (expRatio * (maxCatchRate - minCatchRate))

	rand.Seed(time.Now().UnixNano())
	roll := rand.Float64()

	if roll < catchRate {
		fmt.Printf("%s was caught!\n", pokemonName)
		c.pokedex[pokemonName] = data // add to pokedex
	} else {
		fmt.Printf("%s escaped!\n", pokemonName)
	}

	return nil

}

func main() {
	cfg := &config{
		nextURL: nil,
		prevURL: nil,
		cache:   pokecache.NewCache(time.Minute * 5),
		pokedex: map[string]PokemonInfo{},
	}

	commands := map[string]cliCommand{
		"exit": {
			name:        "exit",
			description: "Exit the Pokedex",
			callback:    commandExit,
		},
		"help": {
			name:        "help",
			description: "Displays a help message",
			callback:    commandHelp,
		},
		"map": {
			name:        "map",
			description: "Display next 20 location areas",
			callback:    commandMap,
		},
		"mapb": {
			name:        "mapb",
			description: "Display previous 20 location areas",
			callback:    commandMapb,
		},
		"explore": {
			name:        "explore",
			description: "Explore a given location area",
			callback:    commandExplore,
		},
		"catch": {
			name:        "catch",
			description: "Attempt to catch a pokemon and add it to your pokedex",
			callback:    commandCatch,
		},
	}

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("Pokedex > ")
		scanner.Scan()
		text := scanner.Text()
		cleaned := cleanInput(text)

		if len(cleaned) == 0 {
			continue
		}

		command, ok := commands[cleaned[0]]
		if !ok {
			fmt.Println("Unknown command")
			continue
		}

		if err := command.callback(cfg, cleaned[1:]); err != nil {
			fmt.Println(err)
		}
	}
}
