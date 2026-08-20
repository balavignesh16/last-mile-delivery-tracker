# Last-Mile Delivery Tracker
## Complete Project Blueprint, Architecture, Modules & Testing Plan

> **Project type:** Internship assignment / production-style delivery management platform  
> **Primary stack:** Go + React + PostgreSQL  
> **Architecture:** Modular Monolith  
> **API style:** REST + OpenAPI  
> **Repository:** Public GitHub repository on `main`

---

# 1. Project Overview

## 1.1 Problem Statement

Logistics operations require several coordinated capabilities:

- Dynamic delivery pricing
- Pickup/drop zone detection
- B2B/B2C-specific pricing
- COD surcharge handling
- Intelligent delivery-agent assignment
- Delivery status tracking
- Immutable tracking history
- Failed-delivery handling
- Customer rescheduling
- Agent reassignment
- Customer notifications
- Admin configuration and operational control

The platform will provide these capabilities through three role-based interfaces:

```text
Customer
    |
    +-- Create order
    +-- View delivery charge
    +-- Track order
    +-- Reschedule failed delivery

Delivery Agent
    |
    +-- View assigned deliveries
    +-- Update delivery status
    +-- Maintain availability/location

Admin
    |
    +-- Manage zones
    +-- Configure rate cards
    +-- Manage agents
    +-- Assign/auto-assign orders
    +-- View/filter orders
    +-- Override order status
```

The assignment specifically requires the output to include an order with an auto-calculated charge, agent assignment, status tracking, and notifications.

---

# 2. Important Assignment Requirements

The implementation must support:

1. Pickup and drop addresses
2. Package dimensions: Length × Breadth × Height
3. Actual package weight
4. Order type: B2B or B2C
5. Payment type: Prepaid or COD
6. Automatic charge calculation
7. Pickup/drop zone detection
8. Volumetric weight:
   `L × B × H ÷ 5000`
9. Chargeable weight as the higher of actual and volumetric weight
10. Correct B2B/B2C intra-zone/inter-zone rate-card selection
11. COD surcharge when applicable
12. Price displayed before order confirmation
13. Manual agent assignment
14. Automatic nearest-available-agent assignment
15. Delivery status lifecycle
16. Immutable tracking history
17. Timestamp and actor for every status change
18. Failed-delivery notification
19. Customer rescheduling
20. Agent reassignment after rescheduling
21. Customer tracking timeline
22. Email notifications on status changes
23. Email/SMS integration
24. Admin order filtering by status/zone/agent
25. Admin status override
26. Role-based authentication
27. Backend API + frontend + database

These requirements are the core of the project. The system should not hard-code business configuration that the assignment says must be admin-configurable.

---

# 3. Evaluation Strategy

The assignment's evaluation focus is:

- Rate calculation engine design and correctness
- Auto-assignment logic and agent availability modelling
- Order status lifecycle and immutable tracking history
- Database schema and data modelling
- API design and code structure
- Documentation

Because many candidates may have the same assignment, the repository will be designed for fast verification.

## Evaluation principle

Every major requirement should have:

```text
Requirement
    ↓
Implementation module
    ↓
Automated tests
    ↓
Documentation
    ↓
Clear README evidence
```

Example:

```text
Rate Calculation
    ↓
backend/internal/rates/
    ↓
backend/tests/rates/
    ↓
docs/rate-engine.md
    ↓
README Evaluation Matrix
```

The evaluator should not have to search through the entire repository to discover whether a requirement exists.

---

# 4. Technology Stack

## Frontend

- React
- TypeScript
- Vite
- Tailwind CSS

## Backend

- Go
- Chi HTTP router
- REST API
- OpenAPI/Swagger
- JWT authentication
- bcrypt password hashing

## Database

- PostgreSQL
- SQL migrations
- `pgx` for PostgreSQL access

## Testing

- Go `testing` package
- `httptest`
- Unit tests
- Integration tests
- API tests
- End-to-end tests

## Infrastructure

- Docker
- Docker Compose

## Deployment

- Vercel for frontend
- Render/Railway or similar for backend
- Hosted PostgreSQL

## Notifications

- Email provider
- SMS provider

Provider selection should remain configurable through environment variables.

---

# 5. Why Modular Monolith?

The project will use a **modular monolith**, not microservices.

```text
                 Go Backend
                     |
     +---------------+----------------+
     |               |                |
   Orders          Rates           Agents
     |               |                |
 Tracking        Rate Engine      Assignment
     |               |                |
     +---------------+----------------+
                     |
                PostgreSQL
```

The modules remain strongly separated inside one deployable backend.

## Why?

- Simpler deployment
- Easier local setup
- Fewer dependencies
- Easier evaluator setup
- Clear business boundaries
- No unnecessary distributed-system complexity
- Easier testing

The project does not require microservices, Kafka, Kubernetes, GraphQL, or other additional infrastructure.

---

# 6. Overall Architecture

```text
                    +----------------------+
                    |      React UI        |
                    | TypeScript + Vite    |
                    |      Tailwind        |
                    +----------+-----------+
                               |
                            HTTPS
                               |
                         REST / JSON
                               |
                    +----------v-----------+
                    |      Go API          |
                    |       Chi            |
                    +----------+-----------+
                               |
       +-----------------------+-----------------------+
       |                       |                       |
       v                       v                       v
+--------------+       +--------------+       +--------------+
| Auth / RBAC  |       | Order Domain |       | Admin Config |
+--------------+       +------+-------+       +------+-------+
                              |                      |
                 +------------+------------+         |
                 |            |            |         |
                 v            v            v         v
              Rates       Tracking     Assignment   Zones
                 |            |            |         |
                 +------------+------------+---------+
                              |
                       +------v------+
                       | PostgreSQL  |
                       +-------------+

                              |
                     +--------+--------+
                     |                 |
                     v                 v
                  Email              SMS
```

---

# 7. Core Modules

The project is divided into 12 major modules.

```text
M01  Foundation & Infrastructure
M02  Authentication & RBAC
M03  User & Agent Management
M04  Zone Management
M05  Rate Configuration
M06  Rate Calculation Engine
M07  Order Management
M08  Tracking & Order Lifecycle
M09  Assignment Engine
M10  Failed Delivery & Rescheduling
M11  Notification Service
M12  Dashboards & Evaluation Layer
```

Testing and documentation are cross-cutting concerns across all modules.

---

# 8. Module M01 — Foundation & Infrastructure

## Purpose

Create the technical foundation before implementing business logic.

## Responsibilities

- Go project initialization
- React project initialization
- PostgreSQL connection
- Environment configuration
- HTTP server
- Routing
- Error handling
- Logging
- Request validation foundation
- Health endpoint
- Testing foundation
- Docker development environment
- Git configuration

## Initial endpoint

```text
GET /health
```

Expected:

```json
{
  "status": "ok"
}
```

## Testing

- Server starts successfully
- Health endpoint returns 200
- Invalid route returns appropriate error
- Database connection succeeds
- Application handles missing configuration correctly

## Completion condition

The project can be cloned and started from a clean environment without business modules.

---

# 9. Module M02 — Authentication & RBAC

## Roles

```text
CUSTOMER
DELIVERY_AGENT
ADMIN
```

## Responsibilities

- Customer registration
- Login
- Password hashing
- JWT generation
- Authentication middleware
- Role authorization
- Current-user endpoint

## Example API

```text
POST /api/v1/auth/register
POST /api/v1/auth/login
GET  /api/v1/auth/me
```

## Authorization model

```text
Request
   |
   v
JWT verification
   |
   v
Authenticated user
   |
   v
Role check
   |
   +-- CUSTOMER
   +-- DELIVERY_AGENT
   +-- ADMIN
```

## Tests

### Unit

- Password hashing
- Password comparison
- JWT creation
- JWT validation
- Role checking

### API

- Successful registration
- Duplicate registration rejected
- Valid login
- Invalid password rejected
- Missing token rejected
- Invalid token rejected
- Customer cannot access admin endpoint
- Agent cannot access admin-only endpoint

---

# 10. Module M03 — User & Agent Management

## Customer

Store:

- User identity
- Contact information
- Profile information

## Delivery Agent

Store:

- Agent identity
- Contact information
- Availability
- Current location
- Current zone

## Agent availability

```text
AVAILABLE
BUSY
OFFLINE
```

## Location model

Support:

```text
latitude
longitude
zone
```

The implementation can use zone-based assignment when precise coordinates are unavailable and geographic distance when coordinates are available.

## Tests

- Create agent
- Update agent availability
- Update location
- Prevent invalid availability transitions
- Retrieve agent profile
- Retrieve eligible agents

---

# 11. Module M04 — Zone Management

## Purpose

Allow administrators to configure delivery zones.

## Responsibilities

- Create zone
- Update zone
- Activate/deactivate zone
- Create/manage areas
- Assign area to zone
- Resolve address/area to zone

## Concept

```text
Address
   |
   v
Area
   |
   v
Zone
```

## Example

```text
Zone A
  |
  +-- Area 1
  +-- Area 2
  +-- Area 3

Zone B
  |
  +-- Area 4
  +-- Area 5
```

## Critical rule

Zone mapping must not be hard-coded inside the rate engine.

It should be database-driven.

## Tests

- Create zone
- Create area
- Assign area to zone
- Resolve area to zone
- Reject unknown area
- Prevent invalid zone relationships
- Verify intra-zone detection
- Verify inter-zone detection

---

# 12. Module M05 — Rate Configuration

This module represents the **admin configuration side** of pricing.

It is separate from the rate calculation engine.

## Configuration categories

```text
B2B Intra-zone
B2B Inter-zone

B2C Intra-zone
B2C Inter-zone

COD surcharge
```

Weight slabs can be represented as configurable rate ranges.

Example demo configuration:

```text
0-5 kg
5-10 kg
10-15 kg
15-20 kg
```

Actual rates are configuration values, not business rules hard-coded into source code.

## Tests

- Create rate card
- Update rate card
- Retrieve rate card
- Activate/deactivate rate
- Reject overlapping weight slabs
- Reject invalid weights
- Verify correct rate card lookup

---

# 13. Module M06 — Rate Calculation Engine

## This is a flagship module.

The engine transforms order/package information into a delivery quote.

## Input

```text
Pickup address
Drop address
Length
Breadth
Height
Actual weight
Order type
Payment type
```

## Step 1 — Detect pickup zone

```text
Pickup Address
     |
     v
Area
     |
     v
Pickup Zone
```

## Step 2 — Detect drop zone

```text
Drop Address
     |
     v
Area
     |
     v
Drop Zone
```

## Step 3 — Determine route type

```text
Pickup Zone == Drop Zone
        |
       YES
        v
   INTRA-ZONE

Pickup Zone != Drop Zone
        |
       YES
        v
   INTER-ZONE
```

## Step 4 — Calculate volumetric weight

Required formula:

```text
Volumetric Weight =
(L × B × H) / 5000
```

Example:

```text
50 × 40 × 30
--------------
    5000

= 12 kg
```

## Step 5 — Calculate chargeable weight

```text
Chargeable Weight =
max(actual weight, volumetric weight)
```

Example:

```text
Actual       = 8 kg
Volumetric   = 12 kg

Chargeable   = 12 kg
```

## Step 6 — Select rate card

Use:

```text
Order Type
    +
Zone Relationship
    +
Chargeable Weight
```

Possible combinations:

```text
B2B + Intra
B2B + Inter
B2C + Intra
B2C + Inter
```

## Step 7 — Apply base rate

Rate is read from admin-configured rate cards.

## Step 8 — Apply COD surcharge

If:

```text
Payment Type == COD
```

then:

```text
Final Charge =
Base Charge + COD Surcharge
```

Otherwise:

```text
Final Charge =
Base Charge
```

## Complete flow

```text
Input
 |
 +-- Zone detection
 |
 +-- Volumetric weight
 |
 +-- Chargeable weight
 |
 +-- Intra/inter classification
 |
 +-- B2B/B2C rate-card lookup
 |
 +-- Base charge
 |
 +-- COD surcharge
 |
 v
Final quote
```

## Important

The quote must be calculated and shown **before the customer confirms the order**.

## Rate-engine tests

### Unit tests

- Volumetric weight calculation
- Actual > volumetric
- Volumetric > actual
- Equal actual/volumetric
- Intra-zone detection
- Inter-zone detection
- B2B rate selection
- B2C rate selection
- COD surcharge
- Prepaid calculation
- Invalid dimensions
- Invalid weight
- Missing rate card
- Missing zone

### Integration tests

- Address → zone → rate card → quote
- Database-configured rate changes affect quote
- COD configuration affects quote
- B2B/B2C configuration affects quote

### Golden test scenarios

Maintain fixed known scenarios with expected outputs so future changes cannot silently break pricing.

---

# 14. Module M07 — Order Management

## Responsibilities

- Quote order
- Create order
- Confirm order
- Retrieve order
- List orders
- Filter orders
- Admin-created orders
- Customer-created orders

## Order creation flow

```text
Customer
   |
   v
Enter delivery information
   |
   v
Request quote
   |
   v
Rate Engine
   |
   v
Display charge
   |
   v
Customer confirms
   |
   v
Create order
```

## Important separation

The quote calculation and final order creation should be separate operations.

Example:

```text
POST /api/v1/orders/quote
```

then:

```text
POST /api/v1/orders
```

The second request should confirm the order based on valid information rather than blindly trusting a client-supplied price.

## Tests

- Create valid order
- Reject invalid order
- Quote before confirmation
- Customer can create own order
- Admin can create order for customer
- Unauthorized user cannot create for another customer
- Order references valid customer
- Order references valid zones
- Order contains valid package information

---

# 15. Module M08 — Tracking & Order Lifecycle

## State machine

```text
CREATED
   |
   v
ASSIGNED
   |
   v
PICKED_UP
   |
   v
IN_TRANSIT
   |
   v
OUT_FOR_DELIVERY
   |
   +------------+
   |            |
   v            v
DELIVERED     FAILED
                  |
                  v
             RESCHEDULED
                  |
                  v
               ASSIGNED
```

The implementation must prevent invalid status jumps.

Example:

```text
CREATED -> DELIVERED
```

should be rejected.

## Immutable tracking event

Every status transition creates a tracking event containing:

```text
event_id
order_id
previous_status
new_status
actor_id
timestamp
metadata
```

Tracking history should not have normal application-level update/delete operations.

## Example

```text
Order #1001

10:00 CREATED
     Actor: Customer

10:05 ASSIGNED
     Actor: Admin

11:00 PICKED_UP
     Actor: Agent

11:30 IN_TRANSIT
     Actor: Agent

13:00 OUT_FOR_DELIVERY
     Actor: Agent

14:10 DELIVERED
     Actor: Agent
```

## Tests

- Valid transitions
- Invalid transitions
- Actor captured
- Timestamp captured
- Event generated on every transition
- Tracking order preserved
- Tracking events cannot be edited through normal API
- Delivered order cannot return to transit
- Failed order enters failure workflow

---

# 16. Module M09 — Assignment Engine

## Manual assignment

Admin selects:

```text
Order
+
Agent
```

Then assignment is created.

## Automatic assignment

Input:

```text
Order pickup location
Available agents
Agent locations
Agent zones
```

## Algorithm

```text
Order
  |
  v
Pickup location / zone
  |
  v
Find available agents
  |
  v
Filter eligible candidates
  |
  v
Determine distance/zone suitability
  |
  v
Rank candidates
  |
  v
Select best candidate
  |
  v
Create assignment
  |
  v
Mark agent BUSY
```

## Candidate filtering

Potential filters:

```text
Agent is AVAILABLE
Agent is active
Agent is eligible for delivery
Agent has usable location/zone information
```

## Distance

When coordinates exist, calculate geographic distance.

When only zone information is available, use zone suitability.

## Tests

### Unit

- Unavailable agents excluded
- Busy agents excluded
- Offline agents excluded
- Same-zone agent preferred when appropriate
- Nearest eligible agent selected
- Tie handling deterministic
- No eligible agent handled safely

### Integration

- Order + agents + database
- Assignment creates database record
- Agent becomes busy
- Assignment produces tracking event if appropriate

### Edge cases

- No agents available
- All agents busy
- Agent location missing
- Multiple agents at same distance
- Agent becomes unavailable during assignment

---

# 17. Module M10 — Failed Delivery & Rescheduling

## Failure flow

```text
OUT_FOR_DELIVERY
       |
       v
     FAILED
       |
       +----> Record failure reason
       |
       +----> Record delivery attempt
       |
       +----> Notify customer
       |
       v
Customer chooses reschedule
       |
       v
Reschedule request
       |
       v
New delivery date
       |
       v
Agent reassignment
       |
       v
New delivery attempt
```

## Delivery attempts

Each attempt should capture:

```text
attempt_id
order_id
agent_id
attempt_number
status
failure_reason
started_at
completed_at
```

## Reschedule

Store:

```text
requested_date
requested_by
requested_at
reason
status
```

## Tests

- Failed status requires valid failure handling
- Failure reason recorded
- Customer notified
- Reschedule request captured
- Invalid reschedule date rejected
- New assignment generated
- Attempt number increments
- Tracking history includes failure/reschedule
- Previously assigned agent is not incorrectly reused when reassignment is required

---

# 18. Module M11 — Notification Service

## Design

The order domain should not directly depend on a specific email/SMS provider.

Instead:

```text
Order event
    |
    v
Notification Service
    |
    +----> Email Provider
    |
    +----> SMS Provider
```

## Events

```text
ORDER_CREATED
AGENT_ASSIGNED
PICKED_UP
IN_TRANSIT
OUT_FOR_DELIVERY
DELIVERED
FAILED
RESCHEDULED
```

## Provider abstraction

```text
NotificationProvider
    |
    +-- EmailProvider
    +-- SmsProvider
```

This allows provider replacement without rewriting business logic.

## Tests

- Notification generated for status change
- Email provider invoked
- SMS provider invoked where applicable
- Provider failure handled safely
- Duplicate notifications prevented where required
- Mock providers used in tests
- No credentials stored in source code

---

# 19. Module M12 — Dashboards & Evaluation Layer

## Customer Dashboard

```text
Dashboard
 |
 +-- Create Order
 +-- My Orders
 +-- Order Details
 +-- Tracking Timeline
 +-- Reschedule Failed Order
```

## Agent Dashboard

```text
Dashboard
 |
 +-- Assigned Deliveries
 +-- Delivery Details
 +-- Update Status
 +-- Availability
 +-- Location
```

## Admin Dashboard

```text
Dashboard
 |
 +-- Order statistics
 +-- Orders
 +-- Agents
 +-- Zones
 +-- Areas
 +-- Rate cards
 +-- Assignment
 +-- Status override
```

## Required admin filters

```text
Status
Zone
Agent
```

---

# 20. API Structure

Use versioned REST endpoints.

```text
/api/v1
```

## Auth

```text
POST /auth/register
POST /auth/login
GET  /auth/me
```

## Users

```text
GET /users/me
PUT /users/me
```

## Agents

```text
GET  /agents
GET  /agents/:id
PUT  /agents/:id/availability
PUT  /agents/:id/location
```

## Zones

```text
POST /zones
GET  /zones
GET  /zones/:id
PUT  /zones/:id
POST /zones/:id/areas
```

## Rates

```text
POST /rates
GET  /rates
GET  /rates/:id
PUT  /rates/:id
```

## Orders

```text
POST /orders/quote
POST /orders
GET  /orders
GET  /orders/:id
```

## Assignment

```text
POST /orders/:id/assign
POST /orders/:id/auto-assign
```

## Tracking

```text
GET  /orders/:id/tracking
POST /orders/:id/status
```

## Rescheduling

```text
POST /orders/:id/reschedule
GET  /orders/:id/reschedules
```

## API documentation

Expose OpenAPI documentation for evaluator-friendly API inspection.

---

# 21. Database Model

Core entities:

```text
users
customers
delivery_agents

areas
zones
rate_cards

orders
packages

order_assignments
tracking_events

delivery_attempts
reschedule_requests

agent_locations
notifications
```

## High-level relationships

```text
USER
 |
 +---- CUSTOMER
 |
 +---- DELIVERY_AGENT
 |
 +---- ADMIN

AREA
 |
 v
ZONE
 |
 v
RATE_CARD

CUSTOMER
 |
 v
ORDER
 |
 +---- PACKAGE
 |
 +---- ASSIGNMENT
 |
 +---- TRACKING_EVENT
 |
 +---- DELIVERY_ATTEMPT
 |
 +---- RESCHEDULE_REQUEST
 |
 +---- NOTIFICATION

AGENT
 |
 +---- LOCATION
 +---- ASSIGNMENTS
 +---- DELIVERY_ATTEMPTS
```

---

# 22. Repository Structure

```text
last-mile-delivery-tracker/
|
├── README.md
├── LICENSE
├── .gitignore
├── .env.example
├── docker-compose.yml
|
├── backend/
│   ├── go.mod
│   ├── go.sum
│   |
│   ├── cmd/
│   │   └── server/
│   │       └── main.go
│   |
│   ├── internal/
│   │   ├── auth/
│   │   ├── users/
│   │   ├── agents/
│   │   ├── zones/
│   │   ├── rates/
│   │   ├── orders/
│   │   ├── tracking/
│   │   ├── assignment/
│   │   ├── rescheduling/
│   │   └── notifications/
│   |
│   ├── migrations/
│   |
│   └── tests/
│       ├── unit/
│       ├── integration/
│       └── e2e/
|
├── frontend/
│   ├── package.json
│   ├── vite.config.ts
│   └── src/
│       ├── components/
│       ├── pages/
│       ├── services/
│       ├── hooks/
│       ├── contexts/
│       ├── types/
│       └── utils/
|
├── docs/
│   ├── architecture.md
│   ├── database.md
│   ├── api.md
│   ├── rate-engine.md
│   ├── assignment-engine.md
│   ├── order-lifecycle.md
│   ├── failed-delivery.md
│   ├── notifications.md
│   ├── testing.md
│   └── system-design.md
|
└── scripts/
    └── seed/
```

---

# 23. Testing Strategy

Testing is a first-class part of development.

The rule is:

```text
Implement
   |
   v
Unit tests
   |
   v
Integration tests
   |
   v
Manual verification
   |
   v
Documentation
   |
   v
Git commit
```

---

# 24. Unit Testing

Unit tests verify isolated business rules.

Priority modules:

```text
Rate Engine
Assignment Engine
Order State Machine
Zone Resolution
COD Calculation
Weight Calculation
Authentication
Validation
```

## Example rate tests

```text
actual > volumetric
volumetric > actual
B2B intra
B2B inter
B2C intra
B2C inter
COD
prepaid
invalid dimensions
missing rate
```

## Example state-machine tests

```text
CREATED -> ASSIGNED
ASSIGNED -> PICKED_UP
PICKED_UP -> IN_TRANSIT
IN_TRANSIT -> OUT_FOR_DELIVERY
OUT_FOR_DELIVERY -> DELIVERED
OUT_FOR_DELIVERY -> FAILED
```

and invalid transitions.

---

# 25. Integration Testing

Integration tests verify that modules work together.

Examples:

```text
Zone database
    +
Rate database
    +
Rate engine
    =
Correct quote
```

```text
Order
    +
Available agents
    +
Assignment engine
    =
Correct assignment
```

```text
Order status change
    +
Tracking repository
    +
Notification service
    =
Tracking event + notification
```

---

# 26. API Testing

Use HTTP-level tests.

Test:

```text
POST /auth/login
POST /orders/quote
POST /orders
POST /orders/:id/assign
POST /orders/:id/auto-assign
POST /orders/:id/status
POST /orders/:id/reschedule
GET  /orders/:id/tracking
```

Verify:

- Status codes
- Response bodies
- Validation errors
- Authorization
- Database effects
- State transitions

---

# 27. End-to-End Testing

The most important E2E scenario:

```text
Customer registers
      |
      v
Customer logs in
      |
      v
Creates quote
      |
      v
Sees calculated charge
      |
      v
Confirms order
      |
      v
Order created
      |
      v
Auto assignment
      |
      v
Agent assigned
      |
      v
Agent picks up
      |
      v
In transit
      |
      v
Out for delivery
      |
      v
Delivered
```

Second E2E scenario:

```text
Customer creates order
      |
      v
Agent assigned
      |
      v
Out for delivery
      |
      v
Failed
      |
      v
Customer notified
      |
      v
Customer reschedules
      |
      v
Agent reassigned
      |
      v
Second delivery attempt
      |
      v
Delivered
```

---

# 28. Test Pyramid

```text
                 /\
                /  \
               / E2E\
              /------\
             /  API   \
            /----------\
           / Integration\
          /--------------\
         /  Unit Tests    \
        /------------------\
```

Most tests should be unit tests.

Fewer integration tests.

A smaller number of complete E2E tests.

This keeps the suite fast while still providing confidence.

---

# 29. Evaluation Evidence Matrix

README should contain a table similar to:

| Requirement | Implementation | Test | Documentation |
|---|---|---|---|
| Volumetric weight | `rates/` | Rate unit tests | `rate-engine.md` |
| Chargeable weight | `rates/` | Weight tests | `rate-engine.md` |
| Zone detection | `zones/` | Zone tests | `architecture.md` |
| B2B/B2C pricing | `rates/` | Rate tests | `rate-engine.md` |
| COD surcharge | `rates/` | COD tests | `rate-engine.md` |
| Manual assignment | `assignment/` | API tests | `assignment-engine.md` |
| Auto assignment | `assignment/` | Assignment tests | `assignment-engine.md` |
| Status lifecycle | `tracking/` | State tests | `order-lifecycle.md` |
| Immutable history | `tracking/` | Tracking tests | `order-lifecycle.md` |
| Failed delivery | `rescheduling/` | Workflow tests | `failed-delivery.md` |
| Notifications | `notifications/` | Provider tests | `notifications.md` |
| RBAC | `auth/` | Auth tests | `architecture.md` |
| Database | `migrations/` | Integration tests | `database.md` |
| REST API | API handlers | API tests | `api.md` |

---

# 30. Development Milestones

## Phase 1 — Foundation

```text
M01 Foundation
M02 Authentication & RBAC
```

## Phase 2 — Configuration

```text
M03 Users & Agents
M04 Zones
M05 Rate Configuration
```

## Phase 3 — Core Business Engine

```text
M06 Rate Calculation Engine
M07 Order Management
M08 Tracking & Lifecycle
```

## Phase 4 — Logistics

```text
M09 Assignment Engine
M10 Failed Delivery & Rescheduling
M11 Notifications
```

## Phase 5 — Product Interface

```text
M12 Customer Dashboard
M12 Agent Dashboard
M12 Admin Dashboard
```

## Phase 6 — Quality

```text
Unit testing
Integration testing
API testing
E2E testing
Error handling
Security review
```

## Phase 7 — Release

```text
Deployment
Documentation
README
Clean clone test
Final evaluator audit
GitHub submission
```

---

# 31. Git Workflow

Use small, meaningful commits.

Examples:

```text
chore: initialize go backend

chore: initialize react frontend

feat: add postgres connection

feat: add authentication and RBAC

feat: implement zone management

feat: add configurable rate cards

feat: implement rate calculation engine

test: add rate engine test suite

feat: implement order creation

feat: implement order state machine

test: add lifecycle tests

feat: implement immutable tracking history

feat: implement agent assignment

test: add assignment engine tests

feat: implement failed delivery workflow

feat: implement rescheduling

feat: add notification service

feat: add customer dashboard

feat: add agent dashboard

feat: add admin dashboard

test: add end-to-end delivery flow

docs: add system architecture

docs: add API documentation

docs: finalize setup guide
```

Avoid meaningless commits such as:

```text
update
changes
final
final2
latest
working
```

---

# 32. Documentation Strategy

The repository should contain:

## `README.md`

High-level entry point:

- Project overview
- Features
- Architecture
- Tech stack
- Quick start
- Demo credentials
- API documentation
- Testing
- Deployment
- Evaluation matrix

## `docs/architecture.md`

- Components
- Responsibilities
- Data flow
- Module boundaries

## `docs/database.md`

- ER model
- Tables
- Relationships
- Constraints
- Indexing decisions

## `docs/rate-engine.md`

- Formula
- Chargeable weight
- Zone selection
- Rate-card selection
- COD
- Examples
- Tests

## `docs/assignment-engine.md`

- Candidate filtering
- Availability
- Zone/location
- Distance
- Ranking
- Edge cases

## `docs/order-lifecycle.md`

- State machine
- Valid transitions
- Invalid transitions
- Tracking events

## `docs/failed-delivery.md`

- Failure
- Notification
- Reschedule
- Reassignment
- Delivery attempt

## `docs/testing.md`

- Test strategy
- Commands
- Test categories
- Coverage approach

## `docs/system-design.md`

Keep the required assignment system-design write-up within the assignment's 800-word limit.

---

# 33. Security Rules

Never commit:

```text
.env
API keys
JWT secrets
Database passwords
Email credentials
SMS credentials
```

Commit:

```text
.env.example
```

Example:

```env
DATABASE_URL=
JWT_SECRET=
EMAIL_API_KEY=
SMS_API_KEY=
```

Other security practices:

- Hash passwords
- Validate inputs
- Enforce RBAC
- Do not trust client-provided prices
- Do not expose internal errors
- Protect admin endpoints
- Use parameterized database operations
- Avoid logging secrets

---

# 34. Important Business Rule: Never Trust Client Price

When the customer confirms an order, the frontend should not be allowed to simply send:

```json
{
  "price": 10
}
```

and have the backend accept it.

The backend must calculate/verify the price from:

```text
Package
+
Zones
+
Order type
+
Payment type
+
Rate configuration
```

This prevents a client from manipulating the delivery price.

---

# 35. Important Business Rule: Configuration vs Engine

Keep these separate.

```text
Admin
  |
  v
Rate Configuration
  |
  v
Database
  |
  v
Rate Engine
  |
  v
Quote
```

The engine should not contain business-specific constants such as:

```text
B2C = 100
COD = 30
```

Those are configuration values.

The engine contains the **calculation rules**, not arbitrary pricing data.

---

# 36. Important Business Rule: State Machine

Never allow arbitrary status updates.

The service must validate:

```text
current state
+
requested next state
```

Example:

```text
CREATED -> ASSIGNED       valid
ASSIGNED -> PICKED_UP    valid
CREATED -> DELIVERED     invalid
DELIVERED -> IN_TRANSIT  invalid
```

This is one of the strongest areas for automated tests.

---

# 37. Important Business Rule: Immutable Tracking

The order's current status can change.

The historical events should remain.

```text
Current status:
DELIVERED

History:
CREATED
ASSIGNED
PICKED_UP
IN_TRANSIT
OUT_FOR_DELIVERY
DELIVERED
```

The history provides an audit trail.

---

# 38. Evaluator Demo Scenarios

The README should provide ready-to-run scenarios.

## Scenario A — B2C COD Inter-Zone

```text
1. Login as customer
2. Enter pickup address
3. Enter drop address
4. Enter package dimensions
5. Enter actual weight
6. Select B2C
7. Select COD
8. Request quote
9. Verify calculated charge
10. Confirm order
11. Verify assignment
```

## Scenario B — Automatic Assignment

```text
1. Create multiple agents
2. Make some unavailable
3. Create order
4. Trigger auto-assignment
5. Verify nearest eligible agent
6. Verify agent becomes BUSY
```

## Scenario C — Failed Delivery

```text
1. Open assigned order
2. Pick up
3. Mark in transit
4. Mark out for delivery
5. Mark failed
6. Verify notification
7. Reschedule
8. Verify reassignment
9. Complete second attempt
```

## Scenario D — Admin Rate Change

```text
1. Login as admin
2. Change a rate card
3. Create a new quote
4. Verify new rate is used
```

---

# 39. Seed Data

Provide safe demo data for evaluators.

Example roles:

```text
Admin
Customer
Agent 1
Agent 2
Agent 3
```

Example configuration:

```text
Zone A
Zone B
Zone C
```

Example rate-card combinations:

```text
B2B Intra
B2B Inter
B2C Intra
B2C Inter
```

Passwords and secrets must never be committed as real production credentials.

---

# 40. Final Quality Gate

Before submission:

```text
[ ] Fresh clone works
[ ] Main branch exists
[ ] Repository is public
[ ] Backend starts
[ ] Frontend starts
[ ] Database migrations work
[ ] Seed data works
[ ] Authentication works
[ ] RBAC works
[ ] Zones work
[ ] Rate configuration works
[ ] Rate engine works
[ ] Quote works
[ ] Order creation works
[ ] Assignment works
[ ] Tracking works
[ ] State machine rejects invalid transitions
[ ] Failed delivery works
[ ] Rescheduling works
[ ] Reassignment works
[ ] Notifications work
[ ] Customer dashboard works
[ ] Agent dashboard works
[ ] Admin dashboard works
[ ] Unit tests pass
[ ] Integration tests pass
[ ] API tests pass
[ ] E2E tests pass
[ ] No .env committed
[ ] No secrets committed
[ ] No node_modules committed
[ ] No build artifacts committed
[ ] No editor-specific files committed
[ ] README is complete
[ ] API documentation is complete
[ ] Database documentation is complete
[ ] Rate engine documentation is complete
[ ] Assignment documentation is complete
[ ] System design is within required word limit
[ ] Deployment works
```

---

# 41. Final Architecture Summary

```text
                         LAST-MILE DELIVERY PLATFORM
                                    |
              +---------------------+---------------------+
              |                     |                     |
          CUSTOMER                AGENT                 ADMIN
              |                     |                     |
              +---------------------+---------------------+
                                    |
                              React Frontend
                                    |
                              REST / OpenAPI
                                    |
                              Go API Server
                                    |
        +---------------------------+---------------------------+
        |                           |                           |
       AUTH                     ORDER DOMAIN                CONFIG
        |                           |                           |
       RBAC              +----------+----------+              |
                         |          |          |              |
                       RATES     TRACKING   ASSIGNMENT        |
                         |          |          |              |
                         +----------+----------+--------------+
                                    |
                             RESCHEDULING
                                    |
                             NOTIFICATIONS
                                    |
                               PostgreSQL
```

---

# 42. Final Project Philosophy

The project should not be judged by how many technologies it uses.

It should demonstrate:

```text
Correctness
    +
Clean Architecture
    +
Strong Business Logic
    +
Database Design
    +
Testing
    +
Security
    +
Documentation
    +
Reproducibility
```

The central engineering goal is:

> **Every important requirement must be implemented as a clear business capability, independently testable, documented, and easy for an evaluator to verify.**

The three highest-priority technical areas are:

```text
1. Rate Calculation Engine
2. Auto-Assignment Engine
3. Order Lifecycle + Immutable Tracking
```

These should receive the strongest design, test coverage, documentation, and evaluator evidence.

---

# 43. Development Rule

For every module:

```text
DESIGN
   ↓
IMPLEMENT
   ↓
UNIT TEST
   ↓
INTEGRATION TEST
   ↓
MANUAL VERIFY
   ↓
DOCUMENT
   ↓
COMMIT
   ↓
NEXT MODULE
```

Never build the entire application first and test at the end.

---

# 44. Final Target

The finished project should allow an evaluator to go from:

```text
GitHub Repository
       ↓
README
       ↓
Architecture
       ↓
Requirement
       ↓
Implementation
       ↓
Tests
       ↓
Running Application
```

with minimal friction.

That is the standard we will use throughout the project.

