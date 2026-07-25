// Command migrate applies pending Atlas versioned migrations against the
// configured database. Run manually, never at container boot.
package main

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"

	"github.com/aprxty3/your_persona_controller.git/internal/config"
)

// migrationsDir is where docker/Dockerfile copies the migrations/ folder.
const migrationsDir = "file:///app/migrations"

func main() {
	dbURL := atlasURL()

	log.Println("Applying Atlas migrations...")
	cmd := exec.Command("atlas", "migrate", "apply",
		"--dir", migrationsDir,
		"--url", dbURL,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("atlas migrate apply failed: %v", err)
	}
	log.Println("Migration completed successfully!")
}

// atlasURL builds the postgres URL Atlas expects from the same DB_* env vars the app uses.
func atlasURL() string {
	host := config.EnvOr("DB_HOST", "localhost")
	port := config.EnvOr("DB_PORT", "5432")
	user := config.EnvOr("DB_USER", "postgres")
	pass := config.EnvOr("DB_PASSWORD", "changeme")
	name := config.EnvOr("DB_NAME", "psyche_assessment")
	sslmode := config.EnvOr("DB_SSLMODE", "disable")
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s&search_path=public",
		url.QueryEscape(user), url.QueryEscape(pass), host, port, name, sslmode)
}
