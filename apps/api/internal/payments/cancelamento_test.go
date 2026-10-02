package payments

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAcaoCancelamento(t *testing.T) {
	casos := []struct {
		order OrderResponse
		acao  string
	}{
		{OrderResponse{Status: "paid", Charges: []OrderCharge{{ID: "ch_1", Status: "paid"}}}, cancelamentoPago},
		{OrderResponse{Status: "pending", Charges: []OrderCharge{{ID: "ch_1", Status: "paid"}}}, cancelamentoPago},
		{OrderResponse{Status: "pending", Charges: []OrderCharge{{ID: "ch_1", Status: "pending"}}}, cancelamentoCancelar},
		{OrderResponse{Status: "canceled", Charges: []OrderCharge{{ID: "ch_1", Status: "canceled"}}}, cancelamentoJaCancelado},
		{OrderResponse{Status: "pending", Charges: []OrderCharge{{ID: "ch_1", Status: "processing"}}}, cancelamentoNaoCancelavel},
		{OrderResponse{Status: "pending"}, cancelamentoNaoCancelavel},
	}
	for _, c := range casos {
		if got, _ := acaoCancelamento(c.order); got != c.acao {
			t.Errorf("%+v: got %s want %s", c.order, got, c.acao)
		}
	}
}

func TestCancelCharge(t *testing.T) {
	var metodo, caminho, usuario string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metodo, caminho = r.Method, r.URL.Path
		usuario, _, _ = r.BasicAuth()
		_, _ = w.Write([]byte(`{"id":"ch_1","status":"canceled"}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "sk_test")
	raw, err := c.CancelCharge(context.Background(), "ch_1")
	if err != nil || metodo != http.MethodDelete || caminho != "/charges/ch_1" || usuario != "sk_test" || len(raw) == 0 {
		t.Fatalf("err=%v metodo=%s caminho=%s usuario=%s", err, metodo, caminho, usuario)
	}
	if _, err := c.CancelCharge(context.Background(), " "); err == nil {
		t.Fatal("sem id deveria falhar")
	}
	erro := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"charge cannot be canceled"}`))
	}))
	defer erro.Close()
	if _, err := NewClient(erro.URL, "k").CancelCharge(context.Background(), "ch_1"); err == nil {
		t.Fatal("422 deveria virar erro")
	}
}
