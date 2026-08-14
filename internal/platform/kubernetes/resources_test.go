package kubernetes

import "testing"

func TestSourceValidation(t *testing.T) {
	s := DBVaultSource{Metadata: ObjectMeta{Name: "app"}, Spec: SourceSpec{Engine: "postgres", RepositoryRef: ObjectReference{Name: "repo"}, Connection: DatabaseConnection{Host: "db", Database: "app", Username: "dbvault", PasswordSecretRef: SecretKeyReference{Name: "secret", Key: "password"}}}}
	if err := ValidateSource(s); err != nil {
		t.Fatal(err)
	}
}
