// Atlas configuration — versioned migrations with the GORM structs as the
// single source of truth. Workflow:
//
//   1. Edit the models in internal/infrastructure/persistence/postgres/models.go
//   2. atlas migrate diff <name> --env gorm   # generates migrations/<ts>_<name>.sql
//   3. commit the generated file (this is the schema history)
//   4. deploy → cmd/migrate applies pending migrations (shells out to atlas)
//
// `atlas migrate diff` needs Docker: it spins an ephemeral postgres:16 to
// compute the diff, then tears it down. Nothing here touches prod/staging.

data "external_schema" "gorm" {
  program = [
    "go", "run", "./cmd/atlasloader",
  ]
}

env "gorm" {
  src = data.external_schema.gorm.url

  // Throwaway database Atlas uses to realize + diff the schema. postgres/16
  // matches the major version running in prod/staging.
  dev = "docker://postgres/16/dev?search_path=public"

  migration {
    dir = "file://migrations"
  }

  format {
    migrate {
      diff = "{{ sql . \"  \" }}"
    }
  }
}
