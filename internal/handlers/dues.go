package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/howarthTech/legion-rome-crm/internal/app"
	"github.com/howarthTech/legion-rome-crm/internal/store"
)

// MembersDuesRecord logs a dues payment and marks the member paid up.
func MembersDuesRecord(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		back := fmt.Sprintf("/members/%d", id)

		paidOn := strings.TrimSpace(r.PostForm.Get("paid_on"))
		if paidOn == "" {
			paidOn = time.Now().Format("2006-01-02")
		}
		if _, err := time.Parse("2006-01-02", paidOn); err != nil {
			redirect(w, r, back, "err", "Enter the payment date as YYYY-MM-DD.")
			return
		}
		method := strings.TrimSpace(r.PostForm.Get("method"))
		year := strings.TrimSpace(r.PostForm.Get("membership_year"))
		notes := strings.TrimSpace(r.PostForm.Get("notes"))

		amount, err := parseAmountCents(r.PostForm.Get("amount"))
		if err != nil {
			redirect(w, r, back, "err", "Amount: "+err.Error())
			return
		}

		if _, err := a.Store.RecordDuesPayment(r.Context(), id, paidOn, amount, method, year, notes); err != nil {
			if errors.Is(err, store.ErrMemberNotFound) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		redirect(w, r, back, "ok", "Dues payment recorded — member marked paid up.")
	}
}

// MembersDuesMarkDue flips a member back to "dues due" (e.g. a new membership
// year) without touching their payment history.
func MembersDuesMarkDue(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		if err := a.Store.SetDuesStatus(r.Context(), id, store.DuesDue); err != nil {
			if errors.Is(err, store.ErrMemberNotFound) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		redirect(w, r, fmt.Sprintf("/members/%d", id), "ok", "Marked as dues due.")
	}
}

// MembersDuesDelete removes a single payment record (to fix a mistake).
func MembersDuesDelete(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		pid, err := strconv.ParseInt(r.PathValue("pid"), 10, 64)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		if err := a.Store.DeleteDuesPayment(r.Context(), id, pid); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		redirect(w, r, fmt.Sprintf("/members/%d", id), "ok", "Payment record removed.")
	}
}

// parseAmountCents turns a dollar string ("12", "12.50", "$12.50") into cents.
// Empty input is allowed (amount optional) and returns an invalid NullInt64.
func parseAmountCents(raw string) (sql.NullInt64, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "$")
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	if s == "" {
		return sql.NullInt64{}, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return sql.NullInt64{}, errors.New("enter a dollar amount like 30 or 30.00")
	}
	if f < 0 {
		return sql.NullInt64{}, errors.New("cannot be negative")
	}
	return sql.NullInt64{Int64: int64(math.Round(f * 100)), Valid: true}, nil
}
