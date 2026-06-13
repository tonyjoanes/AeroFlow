// aeroctl is the command-line interface to the AeroFlow platform API.
// It mirrors the structure of tools like kubectl and ppctl: subcommands that
// call the platform-api and render results in a terminal-friendly table.
//
// Usage:
//
//	aeroctl services list
//	aeroctl flights status
//	aeroctl health
//	aeroctl rollout <deployment> --namespace <ns> --image <image>
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var apiAddr string

func main() {
	root := &cobra.Command{
		Use:   "aeroctl",
		Short: "AeroFlow platform CLI",
		Long:  "Inspect and operate the AeroFlow platform via the platform-api.",
	}
	root.PersistentFlags().StringVar(&apiAddr, "api", envOr("AEROFLOW_API", "http://localhost:9000"), "platform-api base URL")

	root.AddCommand(
		servicesCmd(),
		healthCmd(),
		flightsCmd(),
		rolloutCmd(),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// ── aeroctl services ─────────────────────────────────────────────────────────

func servicesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "services",
		Short: "Inspect cluster services",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List deployments across all AeroFlow namespaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServicesList()
		},
	})
	return cmd
}

type servicesSummary struct {
	Namespace   string `json:"namespace"`
	Deployments []struct {
		Name      string `json:"name"`
		Image     string `json:"image"`
		Ready     int    `json:"ready"`
		Desired   int    `json:"desired"`
		Available bool   `json:"available"`
	} `json:"deployments"`
}

func runServicesList() error {
	var summaries []servicesSummary
	if err := get("/api/services", &summaries); err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAMESPACE\tDEPLOYMENT\tREADY\tSTATUS\tIMAGE")
	for _, s := range summaries {
		for _, d := range s.Deployments {
			status := "Ready"
			if !d.Available {
				status = "Degraded"
			}
			image := d.Image
			if len(image) > 40 {
				image = "…" + image[len(image)-39:]
			}
			fmt.Fprintf(w, "%s\t%s\t%d/%d\t%s\t%s\n",
				s.Namespace, d.Name, d.Ready, d.Desired, status, image)
		}
	}
	return w.Flush()
}

// ── aeroctl health ───────────────────────────────────────────────────────────

func healthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Show aggregate cluster health",
		RunE: func(cmd *cobra.Command, args []string) error {
			var result struct {
				Status string `json:"status"`
			}
			if err := get("/api/health", &result); err != nil {
				return err
			}
			icon := "✓"
			if result.Status != "healthy" {
				icon = "✗"
			}
			fmt.Printf("%s cluster is %s\n", icon, result.Status)
			return nil
		},
	}
}

// ── aeroctl flights ──────────────────────────────────────────────────────────

func flightsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "flights",
		Short: "Inspect flight state",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show recent flights tracked by platform-api",
		RunE: func(cmd *cobra.Command, args []string) error {
			// platform-api tracks flights in memory; retrieve via /api/flights
			// (added as a JSON twin of the /flights page).
			var flights []struct {
				Number      string    `json:"number"`
				Origin      string    `json:"origin"`
				Destination string    `json:"destination"`
				Status      string    `json:"status"`
				Gate        string    `json:"gate"`
				Carousel    string    `json:"carousel"`
				UpdatedAt   time.Time `json:"updated_at"`
			}
			if err := get("/api/flights", &flights); err != nil {
				return err
			}
			if len(flights) == 0 {
				fmt.Println("No flights tracked yet.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "FLIGHT\tORIGIN\tDEST\tSTATUS\tGATE\tCAROUSEL\tUPDATED")
			for _, f := range flights {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					f.Number, f.Origin, f.Destination,
					f.Status, f.Gate, f.Carousel,
					f.UpdatedAt.Format("15:04:05"),
				)
			}
			return w.Flush()
		},
	})
	return cmd
}

// ── aeroctl rollout ──────────────────────────────────────────────────────────

func rolloutCmd() *cobra.Command {
	var namespace, image string

	cmd := &cobra.Command{
		Use:   "rollout [deployment]",
		Short: "Patch a Deployment to a new image tag",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := fmt.Sprintf(
				`{"namespace":%q,"deployment":%q,"image":%q}`,
				namespace, args[0], image,
			)
			var result map[string]string
			if err := post("/api/rollout", strings.NewReader(payload), &result); err != nil {
				return err
			}
			fmt.Printf("✓ %s/%s → %s\n", namespace, args[0], image)
			return nil
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "flights", "Kubernetes namespace")
	cmd.Flags().StringVar(&image, "image", "", "New container image (required)")
	_ = cmd.MarkFlagRequired("image")
	return cmd
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

func get(path string, out any) error {
	resp, err := http.Get(apiAddr + path)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s: %s — %s", path, resp.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func post(path string, body io.Reader, out any) error {
	resp, err := http.Post(apiAddr+path, "application/json", body)
	if err != nil {
		return fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s: %s — %s", path, resp.Status, strings.TrimSpace(string(b)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
