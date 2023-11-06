package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	emailVerifier "github.com/AfterShip/email-verifier"
	"github.com/julienschmidt/httprouter"
)

func GetEmailVerification(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	verifier := emailVerifier.NewVerifier()
	ret, err := verifier.Verify(ps.ByName("email"))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if !ret.Syntax.Valid {
		_, _ = fmt.Fprint(w, "email address syntax is invalid")
		return
	}

	bytes, err := json.Marshal(ret)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	_, _ = fmt.Fprint(w, string(bytes))

}

func GetEmailsVerification(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	emailsStr := r.URL.Query().Get("emails")
	verifier := emailVerifier.NewVerifier()
	emails := strings.Split(emailsStr, ",")

	results, err := verifier.VerifyBulk(emails...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(results); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func main() {
	router := httprouter.New()

	router.GET("/v1/:email/verification", GetEmailVerification)
	router.GET("/verify/bulk", GetEmailsVerification)

	log.Fatal(http.ListenAndServe(":8080", router))
}
