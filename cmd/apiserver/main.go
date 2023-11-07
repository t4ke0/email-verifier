package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	emailVerifier "github.com/AfterShip/email-verifier"
	"github.com/julienschmidt/httprouter"
	"github.com/rs/xid"
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

type BulkVerificationResponse struct {
	Results interface{} `json:"results"`
	Errors  []string    `json:"errors"`
}

func GetEmailsVerification(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	emailsStr := r.URL.Query().Get("emails")
	verifier := emailVerifier.NewVerifier().EnableSMTPCheck()
	if err := verifier.EnableAPIVerifier("gmail"); err != nil {
		_, _ = fmt.Fprintf(w, "EnabledAPIVerifier error:  %v", err)
		return
	}
	emails := strings.Split(emailsStr, ",")

	w.Header().Set("Content-Type", "application/json")
	results, err := verifier.VerifyBulk(emails...)
	if err != nil {
		if err := json.NewEncoder(w).Encode(BulkVerificationResponse{
			Results: results,
			Errors:  strings.Split(err.Error(), ","),
		}); err != nil {
			_, _ = fmt.Fprintf(w, "failed to write JSON to the http writer %v", err)
		}
		return
	}

	if err := json.NewEncoder(w).Encode(BulkVerificationResponse{
		Results: results,
	}); err != nil {
		_, _ = fmt.Fprintf(w, "failed to write JSON to the http writer %v", err)
		return
	}
}

// BulkJob
type BulkJob struct {
	Id             string    `json:"job_id"`
	JobStatus      string    `json:"job_status"`
	TotalRecords   int       `json:"total_records"`
	TotalProcessed int       `json:"total_processed"`
	CreatedAt      time.Time `json:"created_at"`
	FinishedAt     time.Time `json:"finished_at"`
}

type StartBulkValidationJobRequest struct {
	Emails []string `json:"emails"`
}

var bulkJobs map[string]*BulkJob
var bulkJobsResults map[string]BulkVerificationResponse

var mtx = &sync.Mutex{}

func StartBulkValidationJob(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	verifier := emailVerifier.NewVerifier().EnableSMTPCheck()
	if err := verifier.EnableAPIVerifier("gmail"); err != nil {
		_, _ = fmt.Fprintf(w, "EnabledAPIVerifier error:  %v", err)
		return
	}
	if r.ContentLength == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Error %v", err)
		return
	}

	var req StartBulkValidationJobRequest

	if err := json.Unmarshal(data, &req); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Error %v", err)
		return
	}

	if len(req.Emails) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	jobId := xid.New().String()
	response := &BulkJob{
		Id:           jobId,
		JobStatus:    "Running",
		TotalRecords: len(req.Emails),
		CreatedAt:    time.Now().Local(),
	}

	mtx.Lock()
	bulkJobs[jobId] = response
	mtx.Unlock()

	go func() {
		var brsp BulkVerificationResponse
		var results []*emailVerifier.Result
		for c := range verifier.VerifyBulkGenerator(req.Emails...) {
			mtx.Lock()
			bulkJobs[jobId].TotalProcessed++
			mtx.Unlock()
			if c.Error != nil {
				brsp.Errors = append(brsp.Errors, c.Error.Error())
				continue
			}

			results = append(results, c.Result)
		}

		brsp.Results = results
		//
		mtx.Lock()
		bulkJobs[jobId].FinishedAt = time.Now().Local()
		bulkJobs[jobId].JobStatus = "Finished"
		bulkJobsResults[jobId] = brsp
		mtx.Unlock()
		//
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(struct {
		JobId string `json:"job_id"`
	}{jobId}); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "error failed to write JSON %v", err)
	}
}

func GetBulkJobDetails(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	jobId := ps.ByName("job_id")
	mtx.Lock()
	v, ok := bulkJobs[jobId]
	mtx.Unlock()
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if err := json.NewEncoder(w).Encode(v); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "error failed to write JSON %v", err)
	}
}

func GetBulkJobResults(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	jobId := ps.ByName("job_id")
	mtx.Lock()
	v, ok := bulkJobsResults[jobId]
	mtx.Unlock()
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if err := json.NewEncoder(w).Encode(v); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "error failed to write JSON %v", err)
	}

	// TODO: delete job details from bulkJobs
}

func main() {
	bulkJobs = make(map[string]*BulkJob)
	bulkJobsResults = make(map[string]BulkVerificationResponse)
	router := httprouter.New()

	router.GET("/v1/:email/verification", GetEmailVerification)
	router.POST("/bulk/job/start", StartBulkValidationJob)
	router.GET("/bulk/job/status/:job_id", GetBulkJobDetails)
	router.GET("/bulk/job/results/:job_id", GetBulkJobResults)

	log.Fatal(http.ListenAndServe(":8080", router))
}
