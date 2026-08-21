package orders

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"lastmiletracker/internal/auth"
	"lastmiletracker/internal/rates"
	"lastmiletracker/internal/server"
	"lastmiletracker/internal/users"
	"lastmiletracker/internal/zones"
)

const timeLayout = time.RFC3339

type orderResponse struct {
	ID         string `json:"id"`
	CustomerID string `json:"customer_id"`
	CreatedBy  string `json:"created_by"`

	OrderType   string `json:"order_type"`
	PaymentType string `json:"payment_type"`

	PickupAreaID     string `json:"pickup_area_id"`
	DropAreaID       string `json:"drop_area_id"`
	PickupZoneID     string `json:"pickup_zone_id"`
	DropZoneID       string `json:"drop_zone_id"`
	ZoneRelationship string `json:"zone_relationship"`

	LengthCM       float64 `json:"length_cm"`
	BreadthCM      float64 `json:"breadth_cm"`
	HeightCM       float64 `json:"height_cm"`
	ActualWeightKG float64 `json:"actual_weight_kg"`

	VolumetricWeightKG float64 `json:"volumetric_weight_kg"`
	ChargeableWeightKG float64 `json:"chargeable_weight_kg"`

	RateCardID   string  `json:"rate_card_id"`
	BaseRate     float64 `json:"base_rate"`
	CODSurcharge float64 `json:"cod_surcharge"`
	FinalAmount  float64 `json:"final_amount"`

	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

func toOrderResponse(o Order) orderResponse {
	return orderResponse{
		ID:         o.ID,
		CustomerID: o.CustomerID,
		CreatedBy:  o.CreatedBy,

		OrderType:   string(o.OrderType),
		PaymentType: string(o.PaymentType),

		PickupAreaID:     o.PickupAreaID,
		DropAreaID:       o.DropAreaID,
		PickupZoneID:     o.PickupZoneID,
		DropZoneID:       o.DropZoneID,
		ZoneRelationship: string(o.ZoneRelationship),

		LengthCM:       o.LengthCM,
		BreadthCM:      o.BreadthCM,
		HeightCM:       o.HeightCM,
		ActualWeightKG: o.ActualWeightKG,

		VolumetricWeightKG: o.VolumetricWeightKG,
		ChargeableWeightKG: o.ChargeableWeightKG,

		RateCardID:   o.RateCardID,
		BaseRate:     o.BaseRate,
		CODSurcharge: o.CODSurcharge,
		FinalAmount:  o.FinalAmount,

		Status:    o.Status,
		CreatedAt: o.CreatedAt.Format(timeLayout),
	}
}

// customerCreateOrderRequest is the only shape a CUSTOMER caller's
// request body may take. There is deliberately no customer_id field —
// not omitted by oversight, structurally absent — so a CUSTOMER
// attempting to set one gets the whole request rejected by
// DisallowUnknownFields (422), the same fail-closed pattern every prior
// module in this project uses for a field that must never be
// client-controlled.
type customerCreateOrderRequest struct {
	PickupAreaID   string   `json:"pickup_area_id"`
	DropAreaID     string   `json:"drop_area_id"`
	OrderType      string   `json:"order_type"`
	PaymentType    string   `json:"payment_type"`
	LengthCM       *float64 `json:"length_cm"`
	BreadthCM      *float64 `json:"breadth_cm"`
	HeightCM       *float64 `json:"height_cm"`
	ActualWeightKG *float64 `json:"actual_weight_kg"`
}

// adminCreateOrderRequest is the only shape an ADMIN caller's request
// body may take — identical to customerCreateOrderRequest plus the one
// field only an admin may legitimately supply: which customer this
// order is for.
type adminCreateOrderRequest struct {
	CustomerID     string   `json:"customer_id"`
	PickupAreaID   string   `json:"pickup_area_id"`
	DropAreaID     string   `json:"drop_area_id"`
	OrderType      string   `json:"order_type"`
	PaymentType    string   `json:"payment_type"`
	LengthCM       *float64 `json:"length_cm"`
	BreadthCM      *float64 `json:"breadth_cm"`
	HeightCM       *float64 `json:"height_cm"`
	ActualWeightKG *float64 `json:"actual_weight_kg"`
}

// CreateOrderHandler handles POST /api/v1/orders (ADMIN + CUSTOMER —
// enforced by the RBAC middleware in routes.go). The request body's
// accepted shape depends on the caller's role (see
// customerCreateOrderRequest/adminCreateOrderRequest); either way, the
// flow is always: validate caller/customer -> build a rates.QuoteInput
// -> rates.CalculateQuote (M06's one authoritative pricing path, never
// re-implemented here) -> persist the returned QuoteResult verbatim ->
// respond with the created order. No client-supplied price is ever
// trusted, and no pricing field is ever computed by this function.
func CreateOrderHandler(ordersRepo Repository, usersRepo users.Repository, zonesRepo zones.Repository, ratesRepo rates.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := auth.IdentityFromContext(r.Context())
		if !ok {
			// Defensive: unreachable in practice, RequireAuth always runs
			// first on this route.
			server.WriteError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			server.WriteError(w, http.StatusUnprocessableEntity, "invalid request body")
			return
		}

		var customerID string
		var quoteInput rates.QuoteInput
		var problem string

		switch identity.Role {
		case users.RoleCustomer:
			var req customerCreateOrderRequest
			dec := json.NewDecoder(bytes.NewReader(body))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&req); err != nil {
				server.WriteError(w, http.StatusUnprocessableEntity, "invalid request body: "+err.Error())
				return
			}
			quoteInput, problem = rates.ValidateQuoteFields(req.PickupAreaID, req.DropAreaID, req.OrderType, req.PaymentType, req.LengthCM, req.BreadthCM, req.HeightCM, req.ActualWeightKG)
			customerID = identity.UserID

		case users.RoleAdmin:
			var req adminCreateOrderRequest
			dec := json.NewDecoder(bytes.NewReader(body))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&req); err != nil {
				server.WriteError(w, http.StatusUnprocessableEntity, "invalid request body: "+err.Error())
				return
			}
			if req.CustomerID == "" {
				server.WriteError(w, http.StatusUnprocessableEntity, "customer_id is required")
				return
			}
			customer, err := usersRepo.FindByID(r.Context(), req.CustomerID)
			if err != nil {
				if errors.Is(err, users.ErrNotFound) {
					server.WriteError(w, http.StatusUnprocessableEntity, "customer_id does not reference an existing user")
					return
				}
				slog.Error("customer lookup failed", "error", err)
				server.WriteError(w, http.StatusInternalServerError, "could not create order")
				return
			}
			if customer.Role != users.RoleCustomer {
				server.WriteError(w, http.StatusUnprocessableEntity, "customer_id must reference a user with role CUSTOMER")
				return
			}
			quoteInput, problem = rates.ValidateQuoteFields(req.PickupAreaID, req.DropAreaID, req.OrderType, req.PaymentType, req.LengthCM, req.BreadthCM, req.HeightCM, req.ActualWeightKG)
			customerID = customer.ID

		default:
			// Defensive: unreachable, RequireRole only permits ADMIN/CUSTOMER
			// on this route.
			server.WriteError(w, http.StatusForbidden, "insufficient role for this operation")
			return
		}

		if problem != "" {
			server.WriteError(w, http.StatusUnprocessableEntity, problem)
			return
		}

		quote, err := rates.CalculateQuote(r.Context(), zonesRepo, ratesRepo, quoteInput)
		if err != nil {
			if status, message, ok := rates.MapQuoteError(err); ok {
				server.WriteError(w, status, message)
				return
			}
			slog.Error("order pricing calculation failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not calculate order pricing")
			return
		}

		created, err := ordersRepo.CreateOrder(r.Context(), CreateOrderInput{
			CustomerID: customerID,
			CreatedBy:  identity.UserID,
			Quote:      quote,
		})
		if err != nil {
			slog.Error("order creation failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not create order")
			return
		}

		server.WriteJSON(w, http.StatusCreated, toOrderResponse(created))
	}
}

// ListOrdersHandler handles GET /api/v1/orders (ADMIN + CUSTOMER).
// ADMIN sees every order; CUSTOMER sees only their own — the
// distinction is made here, not left to the client to request, so
// there is no query parameter a CUSTOMER could use to see another
// customer's orders.
func ListOrdersHandler(repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := auth.IdentityFromContext(r.Context())
		if !ok {
			server.WriteError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		var list []Order
		var err error
		if identity.Role == users.RoleAdmin {
			list, err = repo.ListAllOrders(r.Context())
		} else {
			list, err = repo.ListOrdersForCustomer(r.Context(), identity.UserID)
		}
		if err != nil {
			slog.Error("order listing failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not list orders")
			return
		}

		responses := make([]orderResponse, len(list))
		for i, o := range list {
			responses[i] = toOrderResponse(o)
		}
		server.WriteJSON(w, http.StatusOK, responses)
	}
}

// GetOrderHandler handles GET /api/v1/orders/{id} (ADMIN + CUSTOMER).
// ADMIN may retrieve any order; CUSTOMER only their own — a CUSTOMER
// requesting another customer's order id gets 404, never 403, so this
// endpoint never confirms that an order exists under an id the caller
// doesn't own. Same ownership/IDOR pattern as M04's area-vs-zone and
// M05's slab-vs-rate-card path checks.
func GetOrderHandler(repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := auth.IdentityFromContext(r.Context())
		if !ok {
			server.WriteError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		id := chi.URLParam(r, "id")
		order, err := repo.FindOrderByID(r.Context(), id)
		if err != nil {
			if errors.Is(err, ErrOrderNotFound) {
				server.WriteError(w, http.StatusNotFound, "order not found")
				return
			}
			slog.Error("order lookup failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not load order")
			return
		}

		if identity.Role == users.RoleCustomer && order.CustomerID != identity.UserID {
			server.WriteError(w, http.StatusNotFound, "order not found")
			return
		}

		server.WriteJSON(w, http.StatusOK, toOrderResponse(order))
	}
}
