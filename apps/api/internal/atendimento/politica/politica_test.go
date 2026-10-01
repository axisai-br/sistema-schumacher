package politica

import (
	"strings"
	"testing"
)

func TestTextoEVersao(t *testing.T) {
	if !strings.Contains(Texto(), "Shabas") || !strings.Contains(Texto(), "+55 49 9886-2222") {
		t.Fatal("politica incompleta")
	}
	if len(Versao()) != 8 || Versao() != Versao() {
		t.Fatal("versao")
	}
}
