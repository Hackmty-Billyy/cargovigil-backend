# Radar en vivo — Inteligencia de despacho (capa de apoyo)

Handoff backend → frontend. Prefijo `/logistics`, 6 endpoints, migraciones `000022`/`000023` aplicadas. No es uno de los 5 módulos del documento original — es la fachada de lectura/orquestación que el dashboard (mapa, radar, programador de viajes) necesita, construida sobre los módulos 2 y 3 que ya existían.

---

## 1. Qué hace este módulo

No tiene reglas de negocio propias. Es una capa que:

1. **Enriquece el viaje** con lo que el mapa necesita — posición actual, avance %, si está detenido — que no vivía en ningún otro módulo.
2. **Combina módulo 2 y módulo 3** en una sola respuesta para responder "¿conviene programar este viaje?" antes de comprometerlo (`POST /trips/projection`).
3. **Simula telemetría** que no existe todavía: no hay GPS real ni lo va a haber pronto, así que `POST /trips/advance-simulation` mueve los viajes un paso (~4 h simuladas) y, cuando algo "pasa" en el camino, **lo registra de verdad** en el módulo 2 (una fricción real, que consume el colchón real) en vez de solo animar un punto en el mapa.
4. **Es el único lugar donde se crean viajes** desde el frontend. `POST /logistics/trips` delega en `/routecost` por dentro — mismo validador de tenancy, mismo colchón automático — así que un viaje nacido en el programador es idéntico a uno dado de alta directo en el módulo 2. No dupliques el formulario de alta en otra pantalla.

---

## 2. Las tres piezas

| Pieza | Qué es |
|---|---|
| **Trip** (enriquecido) | Las columnas reales de `trips` + posición/avance en vivo (migración 000022/000023) + datos resueltos por join: identificador y tipo de vehículo, origen/destino y coordenadas de la ruta, nombre del cliente. Es de solo lectura salvo por lo que el simulador escribe. |
| **ProjectionResponse** | La respuesta de "qué pasaría si programo este viaje": combustible (módulo 3), riesgo y colchón sugerido (módulo 2), margen neto proyectado y una recomendación en texto. |
| **SimulationResult** | Lo que regresa "Avanzar tiempo": un mensaje humano (`"3 viaje(s) avanzaron · 1 incidente(s) nuevo(s)"`) y la lista completa de viajes ya actualizada — el radar reemplaza su estado con esto, no hace merge parcial. |

---

## 3. Contrato de API

Todo cuelga de `/logistics` y requiere `Authorization: Bearer <access_token>`. El grupo completo está abierto a los tres roles de empresa (admin, operations, finance) — `platform_admin` no entra, igual que en `/routecost`.

| Método | Ruta | Rol | Devuelve |
|---|---|---|---|
| GET | `/logistics/trips` | cualquiera | 200 · `Trip[]` — en tránsito/retrasados primero, luego por fecha de salida desc |
| GET | `/logistics/trips/:id` | cualquiera | 200 · `Trip` · 404 si no existe o es de otra empresa |
| POST | `/logistics/trips/projection` | cualquiera | 200 · `ProjectionResponse` |
| POST | `/logistics/trips` | **admin+ops** | 201 · `Trip` (ya con colchón y posición inicial) |
| POST | `/logistics/trips/advance-simulation` | cualquiera | 200 · `SimulationResult` |
| GET | `/logistics/fuel-indexes` | cualquiera | 200 · `FuelIndex[]` (hasta 50, ve `types/fuel.ts` si ya existe) |

"Avanzar tiempo" está abierto a los tres roles a propósito: es un stand-in de telemetría/demo, no una operación financiera — a diferencia de crear un viaje, que sí compromete dinero.

### Payloads

```jsonc
// POST /logistics/trips/projection — antes de comprometer el viaje
{
  "vehicle_type": "truck",        // truck | ship | plane — decide qué índice de combustible se usa
  "route_id": "uuid",
  "distance_km": 850,              // opcional; si falta o es <= 0 se usa 350 como default
  "cargo_weight_tons": 28.5,
  "agreed_price": 10000,
  "currency": "USD"                // opcional, default "USD"
}

// POST /logistics/trips — mismo shape que /routecost/trips.
// contingency_budget se acepta y se IGNORA: lo calcula routecost desde el
// riesgo de la ruta, la respuesta trae el número real.
{
  "vehicle_id": "uuid", "route_id": "uuid", "client_id": "uuid",
  "contract_id": null,
  "tracking_code": "TRIP-2026-014",
  "cargo_type": "Bobinas de acero",
  "cargo_weight_tons": 28.5,
  "departure_date": "2026-10-01T08:00:00",       // acepta con/sin zona, con/sin segundos, o solo fecha
  "estimated_arrival_date": "2026-10-02T20:00:00",
  "agreed_freight_price": 10000, "currency": "USD",
  "fuel_surcharge_amount": 350,
  "contingency_budget": 0          // se ignora, no lo mandes si puedes evitarlo
}

// POST /logistics/trips/advance-simulation
// Body vacío = avanza TODOS los viajes abiertos de la empresa.
{ "trip_id": "uuid" }             // opcional: avanza solo ese viaje
```

Las fechas de `departure_date`/`estimated_arrival_date` en la creación son más permisivas que en `/routecost` directo: aceptan `RFC3339`, `YYYY-MM-DDTHH:mm:ss`, `YYYY-MM-DDTHH:mm` (lo que manda un `<input type="datetime-local">` sin zona) o solo `YYYY-MM-DD`. Un valor sin zona se interpreta como UTC.

---

## 4. Tipos para pegar

`src/types/logistics.ts`. Reutiliza `TripStatus` de `types/routecost.ts` si ya lo tienes.

```ts
import type { TripStatus } from './routecost';

export interface Trip {
  id: string; company_id: string;
  vehicle_id: string; route_id: string; client_id: string; contract_id: string | null;
  tracking_code: string; cargo_type: string; cargo_weight_tons: number;
  status: TripStatus;
  departure_date: string; estimated_arrival_date: string; actual_arrival_date: string | null;
  agreed_freight_price: number; currency: string;
  fuel_surcharge_amount: number; contingency_budget: number;

  // Seguimiento en vivo (lo escribe el simulador, nunca el cliente)
  current_lat: number | null; current_lng: number | null;
  progress_percentage: number;     // 0–100
  is_stuck: boolean;
  stuck_reason: string | null;     // texto humano, ej. "Retención en aduana"
  estimated_fuel_cost: number;     // en la moneda del viaje
  estimated_loss_risk: number;     // exposición por retraso, acotada al flete — nunca lo supera
  last_simulated_step: number;

  created_at: string; updated_at: string;

  // Resuelto por join, solo lectura
  vehicle_identifier: string; vehicle_type: 'truck' | 'ship' | 'plane';
  route_origin: string; route_destination: string; route_distance_km: number | null;
  origin_lat: number | null; origin_lng: number | null;
  dest_lat: number | null; dest_lng: number | null;
  client_name: string;
}

export interface ProjectionRequest {
  vehicle_type: 'truck' | 'ship' | 'plane';
  route_id: string;
  distance_km?: number;
  cargo_weight_tons: number;
  agreed_price: number;
  currency?: string;
}

export interface ProjectionResponse {
  distance_km: number;
  fuel_type: string; current_fuel_price: number; fuel_unit: string; fuel_source: string;
  estimated_fuel_quantity: number; estimated_fuel_cost: number;
  suggested_contingency: number;
  risk_score: number; average_delay_hours: number; potential_loss_risk: number;
  projected_net_margin: number; projected_margin_pct: number;
  recommendation: string;   // texto ya redactado, listo para mostrar
}

export interface SimulationResult {
  message: string;
  trips: Trip[];   // lista completa — reemplaza el estado, no hagas merge
}
```

### Métodos en el ApiClient

```ts
// ===== Radar en vivo =====
listRadarTrips(token: string) {
  return this.request<Trip[]>('/logistics/trips', {}, token);
}
getRadarTrip(token: string, id: string) {
  return this.request<Trip>(`/logistics/trips/${id}`, {}, token);
}
projectTrip(token: string, payload: ProjectionRequest) {
  return this.request<ProjectionResponse>('/logistics/trips/projection',
    { method: 'POST', body: JSON.stringify(payload) }, token);
}
createRadarTrip(token: string, payload: CreateTripPayload) {
  return this.request<Trip>('/logistics/trips', { method: 'POST', body: JSON.stringify(payload) }, token);
}
advanceSimulation(token: string, tripId?: string) {
  return this.request<SimulationResult>('/logistics/trips/advance-simulation',
    { method: 'POST', body: JSON.stringify(tripId ? { trip_id: tripId } : {}) }, token);
}
```

---

## 5. Reglas que la UI no puede inventar

- **No dupliques el alta de viajes.** `POST /logistics/trips` y `POST /routecost/trips` terminan en el mismo lugar (mismo validador, mismo colchón). Si el programador del radar y la pantalla de Módulo 2 usan formularios distintos, deben apuntar al mismo componente o al menos al mismo tipo de payload — no repliques la lógica de armar el request.
- **`estimated_loss_risk` está topado al flete.** Es una cifra de cabecera para verse junto al precio del viaje; el número real sin techo (para reportes) vive en `trip_frictions.opportunity_cost` del módulo 2, no aquí.
- **El simulador reemplaza el estado completo, no lo parchea.** `SimulationResult.trips` es la lista entera de la empresa después del paso — usa eso para el re-render del radar, no combines con el estado previo campo por campo.
- **Un viaje sin coordenadas de ruta no se debe "teletransportar".** Si `origin_lat`/`dest_lat` vienen `null` (ruta creada sin geocodificar), el punto se queda donde estaba — no lo fuerces a `(0,0)`, se ve como un bug en medio del océano.
- **`is_stuck: true` es el mismo incidente que ya está en el módulo 2.** No inventes un badge de "detenido" independiente del que ya muestra `/routecost/trips/:id/frictions`; `stuck_reason` es literalmente la razón de la fricción abierta.
- **La proyección no compromete nada.** `POST /trips/projection` es de solo lectura, se puede llamar tantas veces como el usuario cambie los campos del formulario antes de decidir agendar.
- **Los mismos errores de `/routecost` pueden aparecer aquí.** Al crear un viaje, un 400 puede venir tanto de validaciones propias de logistics (`vehicle_type` inválido, fechas inválidas) como de routecost por debajo (tracking_code duplicado, referencia inválida, moneda no soportada) — el mensaje en `error` ya viene legible en ambos casos, no necesitas distinguir el origen.

---

## 6. Entorno

Igual que el resto de la API: `Bearer` en el header, `VITE_API_URL`, CORS ya configurado para `localhost:5173`/`3000`. No hay nada nuevo que configurar para este módulo — reutiliza el mismo `ApiClient` y el mismo manejo de 204/`error` que ya tienes.
