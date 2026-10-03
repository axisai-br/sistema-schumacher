package canal

import (
	"reflect"
	"testing"
)

func TestFormatarWhatsApp(t *testing.T) {
	casos := []struct{ entrada, esperado string }{
		{"Prefere **integral** ou **sinal**?", "Prefere *integral* ou *sinal*?"},
		{"**a**,**b**", "*a*,*b*"},
		{"Total: **R$ 1.100,00**.", "Total: *R$ 1.100,00*."},
		{"__obs__ e ~~antigo~~", "_obs_ e ~antigo~"},
		{"## Opções\n1. seg 07:30", "*Opções*\n1. seg 07:30"},
		{"### **Resumo**", "*Resumo*"},
		{"Já em *negrito* fica igual", "Já em *negrito* fica igual"},
		{"Ana (CPF ***725) e Bruno (CPF ***901)", "Ana (CPF ***725) e Bruno (CPF ***901)"},
		{"2 * 3 ** 4", "2 * 3 ** 4"},
		{"nome_do_arquivo__v2", "nome_do_arquivo__v2"},
	}
	for _, c := range casos {
		if got := FormatarWhatsApp(c.entrada); got != c.esperado {
			t.Errorf("FormatarWhatsApp(%q) = %q, esperado %q", c.entrada, got, c.esperado)
		}
	}
}

func TestPartesWhatsAppPix(t *testing.T) {
	cod1 := "00020126580014br.gov.bcb.pix0136eval-1-5204000053039865802BR5913SCHUMACHER TUR6009SAO PAULO62070503***6304ABCD"
	cod2 := "000201PIXpay-2"
	texto := "Reserva feita! ✅ Seguem os PIX (copia e cola):\n\nMonção → Fraiburgo, 05/10: **R$ 100,00**\n" + cod1 +
		"\n\nFraiburgo → Monção, 07/10: R$ 100,00\n" + cod2 + "\n\nTotal a pagar agora: R$ 200,00."
	got := PartesWhatsApp(texto)
	esperado := []string{
		"Reserva feita! ✅ Seguem os PIX (copia e cola):\n\nMonção → Fraiburgo, 05/10: *R$ 100,00*",
		cod1,
		"Fraiburgo → Monção, 07/10: R$ 100,00",
		cod2,
		"Total a pagar agora: R$ 200,00.",
	}
	if !reflect.DeepEqual(got, esperado) {
		t.Fatalf("partes:\n%q\nesperado:\n%q", got, esperado)
	}
	// Codigo no meio da linha e no fim do texto.
	got = PartesWhatsApp("Código: " + cod2)
	if !reflect.DeepEqual(got, []string{"Código:", cod2}) {
		t.Fatalf("inline: %q", got)
	}
	// Sem PIX: uma mensagem so.
	if got := PartesWhatsApp("Oi! **Tudo bem?**"); !reflect.DeepEqual(got, []string{"Oi! *Tudo bem?*"}) {
		t.Fatalf("sem pix: %q", got)
	}
	if got := PartesWhatsApp("  \n "); len(got) != 0 {
		t.Fatalf("vazio: %q", got)
	}
}
