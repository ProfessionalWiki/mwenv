package mediawiki

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ProfessionalWiki/mwenv/internal/project"
)

type Database string

const (
	MariaDB Database = "mariadb"
	SQLite  Database = "sqlite"
)

const (
	AdminUser            = "Admin"
	AdminPassword        = "AdminPassword"
	DatabaseRootPassword = "mwenv"
	SQLiteDir            = "/var/www/data"
	databaseName         = "mediawiki"
)

func ServerURL(port int) string {
	return fmt.Sprintf("http://localhost:%d", port)
}

func ParseDatabase(name string) (Database, error) {
	switch Database(name) {
	case MariaDB, SQLite:
		return Database(name), nil
	}
	return "", fmt.Errorf("unknown database %q; use mariadb or sqlite", name)
}

func ComposerLocal(p project.Project) string {
	var composerLocal struct {
		Extra struct {
			MergePlugin struct {
				Include []string `json:"include"`
			} `json:"merge-plugin"`
		} `json:"extra"`
	}
	composerLocal.Extra.MergePlugin.Include = []string{p.Dir() + "/composer.json"}

	content, _ := json.MarshalIndent(composerLocal, "", "\t")
	return string(content) + "\n"
}

func InstallArgs(p project.Project, database Database, port int) []string {
	args := []string{
		"maintenance/run.php", "install",
		"--server=" + ServerURL(port),
		"--scriptpath=/w",
		"--lang=en",
		"--pass=" + AdminPassword,
		"--skins=Vector",
		"--dbname=" + databaseName,
	}

	if database == SQLite {
		args = append(args, "--dbtype=sqlite", "--dbpath="+SQLiteDir)
	} else {
		args = append(args, "--dbtype=mysql", "--dbserver=db", "--dbuser=root", "--dbpass="+DatabaseRootPassword)
	}

	return append(args, p.Name, AdminUser)
}

func Settings(p project.Project, cookiePrefix string) string {
	var settings strings.Builder

	settings.WriteString(`
# Added by mwenv
$wgArticlePath = '/wiki/$1';
$wgCookiePrefix = ` + phpString(cookiePrefix) + `;
$wgResourceLoaderMaxage['unversioned'] = 0;
$wgShowExceptionDetails = true;
$wgDevelopmentWarnings = true;
$wgEnableUploads = true;
if ( PHP_SAPI !== 'cli' ) {
	$wgDebugLogGroups['exception'] = 'php://stderr';
}

`)

	if p.Type == project.Skin {
		settings.WriteString("wfLoadSkin( " + phpString(p.Name) + " );\n")
		settings.WriteString("$wgDefaultSkin = " + phpString(p.DefaultSkinKey) + ";\n")
	} else {
		settings.WriteString("wfLoadExtension( " + phpString(p.Name) + " );\n")
	}

	return settings.String()
}

func phpString(value string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(value) + "'"
}
