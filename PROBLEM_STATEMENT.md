# Last-Mile Delivery Tracker — Problem Statement

> **Source-faithful Markdown transcription of the supplied assignment PDF.**
>
> This file is intended to preserve the assignment brief as the authoritative project problem statement. It does **not** add implementation architecture or technology decisions.

---

## Objective

Logistics operations involve complex pricing rules, dynamic agent assignment, and reliable customer communication.

Build a delivery management platform where customers and admins can create orders with auto-calculated charges, agents are assigned intelligently, and customers are notified at every step of the delivery journey.

---

## Scope of Work

### Input

The system accepts:

- Pickup & drop address
- Package dimensions (`L × B × H`)
- Actual weight
- Order type (`B2B/B2C`)
- Payment type (`Prepaid/COD`)

### Output

The system produces:

- Order with auto-calculated charge
- Agent assignment
- Status tracking
- Notifications

---

## Functional Requirements

### 1. Admin Zone and Rate Management

Admin manages zones, assigns areas to zones, and configures rate cards:

- Intra-zone rates
- Inter-zone rates
- Separate rates for B2B
- Separate rates for B2C
- COD surcharge per order type

---

### 2. Customer Registration and Order Placement

Customer can:

- Register
- Log in
- Place an order

Admin can also create orders on behalf of a customer.

---

### 3. Automatic Charge Calculation

On order creation, the system:

1. Detects the pickup zone.
2. Detects the drop zone.
3. Calculates volumetric weight:

```text
Volumetric Weight = L × B × H ÷ 5000
```

4. Bills on the higher of:
   - Actual weight
   - Volumetric weight
5. Applies the zone rate from the correct rate card:
   - B2B or B2C
6. Adds COD surcharge if applicable.
7. Shows the charge before the customer confirms the order.

---

### 4. Agent Assignment

Admin can:

- Manually assign a delivery agent to an order.
- Trigger automatic assignment to the nearest available agent.

---

### 5. Delivery Status Updates

The delivery agent updates the order status through:

```text
Picked Up
In Transit
Out for Delivery
Delivered
Failed
```

---

### 6. Failed Delivery and Rescheduling

On failed delivery:

1. Customer receives a notification.
2. Customer can reschedule for a new date.
3. Agent is reassigned for the rescheduled attempt.

---

### 7. Customer Tracking

Customer can view:

- Live order status
- Full tracking timeline

---

### 8. Status Notifications

Email notifications are sent to the customer on every status change.

---

### 9. Admin Order Management

Admin can:

- View all orders
- Filter by status
- Filter by zone
- Filter by agent
- Override any order status

---

# Technical Expectations

## Backend, Frontend, Database and Authentication

The project must include:

- Backend API
- Frontend
- Database
- Role-based authentication

Required roles:

```text
Customer
Delivery Agent
Admin
```

---

## Rate Calculation Engine

The system must provide a rate calculation engine covering:

- Zone detection
- Volumetric weight
- B2B/B2C rate-card lookup
- COD surcharge

The assignment specifies that these must be:

> **All admin-configurable, no hardcoding.**

---

## Auto-Assignment Logic

The system must detect the nearest available agent based on:

- Current location
- Or zone

---

## Order Status Lifecycle and Tracking

The system must provide:

- Order status lifecycle
- Immutable tracking history

Each status change must be logged with:

- Timestamp
- Actor

---

## Failed Delivery Flow

The system must support:

- Failed status
- Customer notification
- Reschedule capture
- Agent reassignment

---

## Email and SMS Integration

The system must provide:

- Email notifications
- SMS integration

The assignment permits any free-tier service for email/SMS integration.

---

# Deliverables

The assignment lists the following deliverables:

## 1. Complete Source Code

A ZIP file containing the complete source code.

## 2. README

The README must include:

- Setup guide
- `.env.example`
- API documentation
- Database schema
- Rate calculation logic explanation

## 3. Hosted Application

A hosted application URL.

The assignment lists examples such as:

- Vercel
- Render
- Railway
- Similar hosting platforms

## 4. System Design Write-up

Maximum length:

**800 words**

It must cover:

- Rate calculation engine
- Zone detection approach
- Auto-assignment logic
- Failed delivery handling

---

# Evaluation Focus

The assignment states that evaluation will focus on:

## 1. Rate Calculation Engine

- Design
- Correctness
- Zone
- Volumetric weight
- B2B/B2C
- COD

## 2. Auto-Assignment

- Assignment logic
- Agent availability modelling

## 3. Order Lifecycle

- Order status lifecycle
- Immutable tracking history

## 4. Database

- Database schema
- Data modelling

## 5. API and Code

- API design
- Code structure

## 6. Documentation

- Quality and completeness of documentation

---

# Required Core Flow

The assignment can be understood as the following end-to-end flow:

```text
Customer / Admin
       |
       v
Create Order
       |
       +--> Pickup Address
       +--> Drop Address
       +--> Dimensions
       +--> Actual Weight
       +--> B2B / B2C
       +--> Prepaid / COD
       |
       v
Detect Zones
       |
       v
Calculate Volumetric Weight
       |
       v
Use Higher of Actual vs Volumetric Weight
       |
       v
Select Correct Rate Card
       |
       v
Apply COD Surcharge if Applicable
       |
       v
Show Charge Before Confirmation
       |
       v
Confirm Order
       |
       v
Assign Delivery Agent
       |
       v
Delivery Status Updates
       |
       +--> Picked Up
       +--> In Transit
       +--> Out for Delivery
       +--> Delivered
       |
       +--> Failed
              |
              v
        Notify Customer
              |
              v
          Reschedule
              |
              v
        Reassign Agent
```

---

# Assignment Requirement Summary

| Area | Required Capability |
|---|---|
| Users | Customer, Delivery Agent, Admin |
| Authentication | Role-based authentication |
| Orders | Customer/admin order creation |
| Pricing | Automatic charge calculation |
| Zones | Pickup/drop zone detection |
| Weight | Actual vs volumetric |
| Volumetric formula | `L × B × H ÷ 5000` |
| Rate cards | B2B/B2C + intra/inter-zone |
| COD | Surcharge when applicable |
| Pricing configuration | Admin-configurable, no hardcoding |
| Assignment | Manual + automatic |
| Auto-assignment | Nearest available agent |
| Status | Picked Up, In Transit, Out for Delivery, Delivered, Failed |
| Tracking | Immutable history |
| Tracking metadata | Timestamp + actor |
| Failed delivery | Notify + reschedule + reassign |
| Customer | Live status + timeline |
| Notifications | Email on status changes |
| Integration | Email + SMS |
| Admin | Order filtering + status override |
| Backend | API |
| Frontend | Application UI |
| Database | Persistent data model |
| Documentation | README, API, DB, rate logic |
| Hosting | Hosted application URL |
| System design | Maximum 800 words |

---

## Source Boundary

This file intentionally does **not** define:

- Programming language
- Framework
- Database technology
- Folder structure
- Module implementation details
- Testing framework
- Deployment architecture beyond the assignment's example hosting platforms
- Specific rate values
- Specific COD surcharge values
- Specific distance algorithm

Those are implementation/design decisions and should be maintained separately from this source-faithful problem statement.
