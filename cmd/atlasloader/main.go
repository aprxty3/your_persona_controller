// Command atlasloader prints the GORM-derived schema as SQL on stdout so Atlas
// can diff it into versioned migrations.
package main

import (
	"fmt"
	"io"
	"os"

	"ariga.io/atlas-provider-gorm/gormschema"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
)

func main() {
	stmts, err := gormschema.New("postgres").Load(
		&postgres.UserModel{},
		&postgres.GuestSessionModel{},
		&postgres.TestResultModel{},
		&postgres.VerificationTokenModel{},
		&postgres.ReferralCodeModel{},
		&postgres.ReferralEventModel{},
		&postgres.DataDeletionRequestModel{},
		&postgres.QuestionModel{},
		&postgres.QuestionTranslationModel{},
		&postgres.AnswerModel{},
		&postgres.InsightTemplateModel{},
		&postgres.PromptAuditLogModel{},
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "atlasloader: failed to load gorm schema: %v\n", err)
		os.Exit(1)
	}
	if _, err := io.WriteString(os.Stdout, stmts); err != nil {
		fmt.Fprintf(os.Stderr, "atlasloader: failed to write schema: %v\n", err)
		os.Exit(1)
	}
}
