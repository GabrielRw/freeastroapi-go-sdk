package freeastro_test

// Examples are compiled by go test (they have no Output comment, so they do not call the API).

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	freeastro "github.com/GabrielRw/freeastroapi-go-sdk"
	"github.com/GabrielRw/freeastroapi-go-sdk/api"
)

func ExampleNewClient() {
	client, err := freeastro.NewClient(
		freeastro.WithAPIKey(os.Getenv("FREEASTRO_API_KEY")),
		freeastro.WithTimeout(30*time.Second),
		freeastro.WithMaxRetries(3),
	)
	if err != nil {
		log.Fatal(err)
	}
	_ = client
}

func ExampleGeoService_Search() {
	client, err := freeastro.NewClient() // reads FREEASTRO_API_KEY
	if err != nil {
		log.Fatal(err)
	}
	cities, err := client.Geo.Search(context.Background(), &api.V2GeoSearchParams{
		Q:     "Paris",
		Limit: freeastro.Ptr(3),
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, city := range cities.Results {
		fmt.Println(city.Name, city.Country, city.Lat, city.Lng, city.Timezone)
	}
}

func ExampleNatalService_Calculate() {
	client, err := freeastro.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	chart, err := client.Natal.Calculate(context.Background(), nil, api.NatalChartRequest{
		Year: 1990, Month: 6, Day: 15,
		Hour: freeastro.Ptr(14), Minute: freeastro.Ptr(30),
		City: freeastro.Ptr("Paris"),
	})
	if err != nil {
		log.Fatal(err)
	}
	// chart is *api.NatalCalculateResponse; optional fields are pointers.
	if chart.Planets != nil {
		for _, p := range *chart.Planets {
			fmt.Println(p.Name, p.Sign)
		}
	}
}

func ExampleVedicService_Panchang() {
	client, err := freeastro.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	panchang, err := client.Vedic.Panchang(context.Background(), api.PanchangRequestV2{
		Year: 2026, Month: 1, Day: 14, Hour: 6, Minute: 0,
		City: freeastro.Ptr("Delhi"),
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", panchang)
}

func ExampleVedicService_KPRender() {
	client, err := freeastro.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	svg, err := client.Vedic.KPRender(context.Background(), api.KPChartRenderRequest{
		Year: 1990, Month: 6, Day: 15, Hour: 14, Minute: 30, City: freeastro.Ptr("Paris"),
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("kp.svg", svg.Data, 0o644); err != nil {
		log.Fatal(err)
	}
}

func ExampleAPIError() {
	client, err := freeastro.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	_, err = client.Geo.Search(context.Background(), &api.V2GeoSearchParams{Q: "Paris"})

	var rateLimited *freeastro.RateLimitError
	var invalid *freeastro.ValidationError
	var apiErr *freeastro.APIError
	switch {
	case errors.As(err, &rateLimited):
		fmt.Println("retry after", rateLimited.RetryAfter)
	case errors.As(err, &invalid):
		fmt.Println("invalid request:", string(invalid.Body))
	case errors.As(err, &apiErr):
		fmt.Println(apiErr.StatusCode, apiErr.Code, apiErr.RequestID)
	case err != nil:
		fmt.Println("network or timeout error:", err)
	}
}

func ExampleWithIdempotencyKey() {
	client, err := freeastro.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	// POST requests are retried only when they carry an idempotency key (or WithRetry(true)).
	sun, err := client.Western.Signs.Sun(context.Background(),
		api.NatalChartRequest{Year: 1990, Month: 6, Day: 15, City: freeastro.Ptr("Paris")},
		freeastro.WithIdempotencyKey("sun-1990-06-15-paris"),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(sun.Sign)
}
