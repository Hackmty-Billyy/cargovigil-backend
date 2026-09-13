# Módulo 2 — Inteligencia de costes operativos y fricciones

Handoff backend → frontend. Prefijo `/routecost`, 19 endpoints, migración `000019` aplicada y probada contra Postgres real.

---

## 1. Qué hace este módulo

Un camión parado en aduana no aparece en ningún estado financiero, pero cuesta dinero. Este módulo registra esos paros (*fricciones*), les pone precio en dos dimensiones distintas, y con el historial de cada ruta calcula cuánta liquidez hay que apartar por viaje.

Lo importante para la UI: **el colchón de contingencia que este módulo calcula se convierte en un gasto pendiente real en tesorería**, así que lo que se muestre aquí tiene que cuadrar con lo que el módulo 1 proyecta en caja.

### Ciclo del dinero reservado

```
POST /trips                 Colchón automático              expenses (tesorería)
operations crea  ────────►  % del perfil de riesgo  ─────►  contingency_reserve   ──► baja la
el viaje                    de la ruta · USD → MXN          status: pending           proyección
                                    ▲                              ▲
                                    │                              │
                            Fricción con costo            Viaje completado
                            consume el colchón            libera el remanente
```

El colchón nunca se suma dos veces: si una fricción cuesta dinero de verdad, ese gasto se cobra *contra* la reserva y la reserva se encoge por el mismo monto. Solo el excedente es dinero nuevo saliendo de la proyección.

---

## 2. Las cuatro entidades

| Entidad | Qué es |
|---|---|
| **Trip** | El viaje. La entidad central de toda la plataforma; antes no existía en la API. Los módulos 3, 4 y 5 van a colgar de aquí. |
| **Friction** | El tiempo muerto: demurrage en puerto, retención en aduana, falla mecánica. Puede estar **abierta** (sigue corriendo) o cerrada. |
| **RouteRiskProfile** | Score de 1.00 a 3.00 y % de colchón sugerido por ruta. Se recalcula solo cada 24 h y al cerrar cada viaje. La UI lo lee, no lo edita. |
| **ContingencyFund** | El colchón, uno por viaje. Guarda lo asignado, lo consumido y lo liberado, más el tipo de cambio con el que se calculó. |

---

## 3. Contrato de API

Todo cuelga de `/routecost` y requiere `Authorization: Bearer <access_token>`. El grupo completo excluye a `platform_admin` (no tiene empresa propia, recibiría 403).

| Método | Ruta | Rol | Devuelve |
|---|---|---|---|
| GET | `/routecost/trips?status=&route_id=` | cualquiera | 200 · `Trip[]` |
| POST | `/routecost/trips` | admin+ops | 201 · `Trip` con colchón ya asignado |
| GET | `/routecost/trips/:id` | cualquiera | 200 · `Trip` |
| PUT | `/routecost/trips/:id` | admin+ops | **204 sin body** — refetch obligatorio |
| DELETE | `/routecost/trips/:id` | admin+ops | 204 |
| PATCH | `/routecost/trips/:id/status` | admin+ops | 200 · `Trip` |
| GET | `/routecost/trips/:id/impact` | cualquiera | 200 · `TripImpact` (el resumen de la vista) |
| GET | `/routecost/trips/:id/frictions` | cualquiera | 200 · `Friction[]` |
| POST | `/routecost/trips/:id/frictions` | admin+ops | 201 · `Friction` |
| GET | `/routecost/frictions?open=true` | cualquiera | 200 · `Friction[]` de toda la empresa |
| PATCH | `/routecost/frictions/:id/close` | admin+ops | 200 · `Friction` |
| GET | `/routecost/risk-profiles` | cualquiera | 200 · `RouteRiskProfile[]` |
| POST | `/routecost/risk-profiles/recalculate?route_id=` | admin+ops | 200 · `RouteRiskProfile[]` |
| GET | `/routecost/contingency` | cualquiera | 200 · `ContingencyFund[]` |
| GET | `/routecost/trips/:id/contingency` | cualquiera | 200 · `ContingencyFund` · 404 si no hay |
| POST | `/routecost/trips/:id/contingency/allocate` | **admin+finance** | 200 · `ContingencyFund` |
| POST | `/routecost/trips/:id/contingency/release` | **admin+finance** | 200 · `ContingencyFund` |
| GET | `/routecost/settings` | cualquiera | 200 · `CostSettings` |
| PUT | `/routecost/settings` | **admin+finance** | 200 · `CostSettings` |

"cualquiera" = los tres roles de empresa (admin, operations, finance).

### Payloads

```jsonc
// POST /routecost/trips — NO mandes contingency_budget, lo calcula el backend
{
  "vehicle_id": "uuid", "route_id": "uuid", "client_id": "uuid",
  "contract_id": null,                    // opcional
  "tracking_code": "TRIP-2026-014",       // único por empresa
  "cargo_type": "Bobinas de acero",       // default "general"
  "cargo_weight_tons": 28.5,
  "departure_date": "2026-10-01T08:00:00Z",
  "estimated_arrival_date": "2026-10-02T20:00:00Z",
  "agreed_freight_price": 10000, "currency": "USD",
  "fuel_surcharge_amount": 350
}

// PATCH /routecost/trips/:id/status
// scheduled | in_transit | delayed | completed | cancelled
{ "status": "in_transit", "actual_arrival_date": null }

// POST /routecost/trips/:id/frictions — sin ended_at queda ABIERTA
{
  "event_type": "port_demurrage",
  "location_name": "Puerto de Manzanillo",
  "started_at": "2026-10-01T14:00:00Z",
  "ended_at": null,
  "cost_impact": 0,                       // en la moneda DEL VIAJE
  "notes": "Espera de atraque"
}

// PATCH /routecost/frictions/:id/close
{ "ended_at": "2026-10-01T18:30:00Z", "cost_impact": 300, "notes": null }

// PUT /routecost/settings — tarifa horaria de inactividad (override)
{ "idle_hourly_rate": 3500, "currency": "MXN" }  // null = derivar del viaje
```

Las fechas se aceptan como RFC3339 completo o como fecha suelta (`2026-10-01`).

### `event_type` — los ocho válidos

```
port_demurrage · customs_delay · traffic_congestion · mechanical_failure
route_deviation · weather_hazard · security_incident · warehouse_detention
```

---

## 4. Tipos para pegar

Nuevo archivo `src/types/routecost.ts`, mismo estilo que `types/company.ts`. Los montos vienen como `number` y las fechas como string ISO.

```ts
export type TripStatus = 'scheduled' | 'in_transit' | 'delayed' | 'completed' | 'cancelled';
export type FrictionEventType =
  | 'port_demurrage' | 'customs_delay' | 'traffic_congestion' | 'mechanical_failure'
  | 'route_deviation' | 'weather_hazard' | 'security_incident' | 'warehouse_detention';
export type FundStatus = 'allocated' | 'partially_consumed' | 'exhausted' | 'released';

export interface Trip {
  id: string; company_id: string;
  vehicle_id: string; route_id: string; client_id: string; contract_id: string | null;
  tracking_code: string; cargo_type: string; cargo_weight_tons: number;
  status: TripStatus;
  departure_date: string; estimated_arrival_date: string; actual_arrival_date: string | null;
  agreed_freight_price: number; currency: string;
  fuel_surcharge_amount: number;
  contingency_budget: number;   // espejo del colchón, en la moneda del viaje
  created_at: string; updated_at: string;
}

export interface Friction {
  id: string; company_id: string; trip_id: string;
  event_type: FrictionEventType; location_name: string | null;
  started_at: string; ended_at: string | null;   // null = sigue abierta
  duration_hours: number;
  cost_impact: number;        // costo de bolsillo (moneda del viaje)
  opportunity_cost: number;   // ingreso que el activo no produjo
  notes: string | null; created_at: string;
}

export interface RouteRiskProfile {
  id: string; company_id: string; route_id: string;
  historical_risk_score: number;              // 1.00 – 3.00
  avg_delay_hours: number;
  suggested_contingency_percentage: number;   // 3 – 25
  incident_count: number; last_calculated_at: string;
}

export interface ContingencyFund {
  id: string; company_id: string; trip_id: string;
  route_risk_score: number; applied_percentage: number;
  base_amount: number; base_currency: string;   // flete y moneda del viaje
  fx_rate: number; reserve_currency: string;    // siempre MXN hoy
  allocated_amount: number; consumed_amount: number; released_amount: number;
  status: FundStatus; reserve_expense_id: string | null;
  calculated_at: string; created_at: string; updated_at: string;
}

export interface TripImpact {
  trip_id: string; currency: string;
  friction_count: number; open_friction_count: number;
  total_idle_hours: number;        // las abiertas cuentan hasta ahora
  direct_cost: number; opportunity_cost: number; total_friction_impact: number;
  idle_hourly_rate: number;
  contingency_fund: ContingencyFund | null;
}

export interface CostSettings {
  company_id: string; idle_hourly_rate: number | null; currency: string; updated_at: string;
}
```

### Métodos en el ApiClient

Se agregan al `src/services/api.ts` que ya existe; `this.request` ya maneja el 204 y el `data.error`.

```ts
// ===== Módulo 2: fricciones y coste operativo =====
listTrips(token: string, q?: { status?: TripStatus; route_id?: string }) {
  const p = new URLSearchParams(q as Record<string, string>).toString();
  return this.request<Trip[]>(`/routecost/trips${p ? `?${p}` : ''}`, {}, token);
}
createTrip(token: string, payload: CreateTripPayload) {
  return this.request<Trip>('/routecost/trips', { method: 'POST', body: JSON.stringify(payload) }, token);
}
changeTripStatus(token: string, id: string, status: TripStatus, actual_arrival_date?: string) {
  return this.request<Trip>(`/routecost/trips/${id}/status`,
    { method: 'PATCH', body: JSON.stringify({ status, actual_arrival_date: actual_arrival_date ?? null }) }, token);
}
getTripImpact(token: string, id: string) {
  return this.request<TripImpact>(`/routecost/trips/${id}/impact`, {}, token);
}
listTripFrictions(token: string, id: string) {
  return this.request<Friction[]>(`/routecost/trips/${id}/frictions`, {}, token);
}
openFriction(token: string, tripId: string, payload: FrictionPayload) {
  return this.request<Friction>(`/routecost/trips/${tripId}/frictions`,
    { method: 'POST', body: JSON.stringify(payload) }, token);
}
closeFriction(token: string, id: string, ended_at: string, cost_impact?: number) {
  return this.request<Friction>(`/routecost/frictions/${id}/close`,
    { method: 'PATCH', body: JSON.stringify({ ended_at, cost_impact: cost_impact ?? null }) }, token);
}
listRiskProfiles(token: string) {
  return this.request<RouteRiskProfile[]>('/routecost/risk-profiles', {}, token);
}
listContingencyFunds(token: string) {
  return this.request<ContingencyFund[]>('/routecost/contingency', {}, token);
}
allocateContingency(token: string, tripId: string) {
  return this.request<ContingencyFund>(`/routecost/trips/${tripId}/contingency/allocate`, { method: 'POST' }, token);
}
```

---

## 5. Plan de implementación

Cinco fases, cada una entregable por su cuenta. El orden importa: la fase 1 desbloquea todo lo demás, porque sin viajes en pantalla no hay dónde colgar fricciones ni colchón.

### Fase 1 — Base

`src/types/routecost.ts` · `src/services/api.ts` · `src/components/Dashboard.tsx`

- Crear `types/routecost.ts` y los métodos del `ApiClient` de arriba.
- Agregar la pestaña **Viajes** en `Dashboard.tsx` junto a las de catálogo, visible para los tres roles de empresa y oculta para `platform_admin` (el patrón `!isPlatformAdmin` ya existe).
- Derivar `isOperations` / `isFinance` de `user.role_name` igual que ya se hace con `isAdmin`; hacen falta en todas las fases siguientes.

### Fase 2 — Listado y alta de viajes

`src/components/routecost/TripsManager.tsx`

- Misma forma que `VehiclesManager`: tabla, modal de alta, estados de carga y error.
- Los selects de vehículo, ruta, cliente y contrato se llenan con los endpoints de catálogo que ya consumes.
- Filtros por `status` y por ruta contra el query string, no en memoria.
- Al crear, la respuesta ya trae `contingency_budget` calculado: muéstralo en el toast de éxito, es el primer momento "wow" del módulo.
- Botones de alta/edición solo para admin y operations; finance ve la tabla en modo lectura.

### Fase 3 — Detalle de viaje con impacto y fricciones

`src/components/routecost/TripDetail.tsx` · `src/components/routecost/FrictionLog.tsx`

- Vista alimentada por `/impact` (resumen arriba) más `/frictions` (bitácora abajo).
- Cuatro cifras en el encabezado: horas muertas, costo directo, pérdida de oportunidad e impacto total — con la moneda del viaje explícita al lado, nunca un símbolo suelto.
- Una fricción con `ended_at: null` se pinta distinto y con contador vivo; es la única parte de la UI que se recalcula sola con `setInterval`.
- Formulario de registro rápido: tipo de evento (los ocho, con etiquetas en español), lugar, inicio y cierre opcional.
- Acción de cerrar fricción pidiendo hora de fin y costo real.
- Cambio de estado del viaje. Al pasar a `completed` hay que refrescar también el colchón y el perfil de riesgo: el backend los movió en la misma operación.

### Fase 4 — Colchón de contingencia

`src/components/routecost/ContingencyPanel.tsx`

- Panel dentro del detalle del viaje: asignado / consumido / liberado, con una barra que muestre cuánto queda.
- Mostrar siempre las dos monedas: `base_amount` en la del viaje y `allocated_amount` en MXN, con el `fx_rate` aplicado a la vista. Es información de auditoría, no un detalle técnico.
- Botones de reasignar y liberar solo para admin y finance. Para operations ni siquiera los pintes: el backend responde 403 y una UI que ofrece lo que no puede hacer es una UI rota.
- Vista agregada de `/contingency`: cuánta liquidez tiene comprometida la empresa ahora mismo.

### Fase 5 — Riesgo por ruta y ajustes

`src/components/routecost/RouteRiskView.tsx` · `src/components/routecost/CostSettingsForm.tsx`

- Tabla de rutas ordenada por score, con el % sugerido, horas promedio de retraso e incidentes.
- El score es un multiplicador de 1.00 a 3.00: codifícalo como escala de tres tramos, no como porcentaje.
- Botón de recalcular (admin y operations) y la fecha de `last_calculated_at` visible, porque si no parece un dato muerto.
- Formulario de tarifa horaria de inactividad. Dejar claro que vacío significa "derivarla del flete de cada viaje", que es el comportamiento por defecto.

---

## 6. Reglas que la UI no puede inventar

- **Dos monedas, no una.** Los viajes están en USD y la tesorería razona en MXN. El backend convierte con un tipo de cambio de configuración y lo congela en cada colchón. Nunca sumes `cost_impact` (moneda del viaje) con `allocated_amount` (MXN).
- **El colchón es automático.** Se asigna solo al crear el viaje. El endpoint `allocate` es para reasignar cuando cambió el riesgo de la ruta, no para el alta. Y no mandes `contingency_budget` en el POST: se ignora.
- **Costo directo ≠ pérdida de oportunidad.** `cost_impact` es lo que le debes a un tercero. `opportunity_cost` es el ingreso que el activo no generó mientras estuvo parado. Son dos columnas distintas y sumarlas como una sola miente.
- **Un viaje cerrado no se reabre.** `completed` y `cancelled` son estados finales: cualquier otro cambio devuelve 400. Deshabilita el control en vez de dejar que el usuario choque contra el error.
- **Una fricción se cierra una vez.** Cerrar una ya cerrada devuelve 400. Y al cerrar, solo el *aumento* de `cost_impact` genera gasto nuevo, para que no se cobre dos veces.
- **El PUT de viaje no devuelve nada.** Responde 204. Después de editar hay que volver a pedir el viaje; no actualices el estado local con lo que mandaste.

---

## 7. Ejemplo verificado contra la base real

Recorrido completo probado con requests reales sobre la empresa CargoVigil Dev. Úsalo como caso de prueba: si tus pantallas reproducen estos números, están bien conectadas.

| Paso | Monto |
|---|---|
| Viaje `TEST-M2-001`, flete acordado | 2,000.00 USD |
| Colchón automático — ruta con score 1.85, 12.35% sugerido | 247.00 USD |
| Reserva que llega a tesorería (tipo de cambio 17.50) | 4,322.50 MXN |
| Fricción abierta 3 h · tarifa derivada 166.67 USD/h | 500.01 USD de oportunidad |
| Se cierra con 100 USD de demurrage → consume colchón | 1,750.00 MXN |
| La reserva pendiente se encoge sola | 2,572.50 MXN |
| **Salida total proyectada — no cambió** | **4,322.50 MXN** |

En un segundo viaje completado a tiempo, el colchón se liberó entero (2,161.25 MXN), la reserva quedó cancelada y el score de la ruta bajó de 1.85 a 1.68, con lo que el % sugerido pasó de 12.35% a 10.48%. Ese es el bucle completo: lo que pasa en la carretera mueve la proyección de caja.

---

## 8. Entorno

- Base URL igual que siempre: `VITE_API_URL` (`apidev.cargovigil.tech` en dev).
- Auth idéntica al módulo 0: `Bearer` en el header, refresh con token opaco rotable. No hay nada nuevo que implementar en sesión.
- CORS ya admite `localhost:5173` y `localhost:3000` con credenciales.
- Contra local el backend corre en `:8080` y Postgres en el contenedor `timescaledb`. Si el backend no levanta, casi siempre es el Postgres nativo de Windows peleando el puerto, no las credenciales.
- Errores: `400` validación con mensaje legible en `error`, `403` rol insuficiente, `404` no existe o es de otra empresa. Las listas vacías llegan como `[]`, nunca `null`.
