# Contexto para continuar CargoVigil Backend — Módulo 2

> **Superado.** Este doc era el handoff para *empezar* el Módulo 2 (`domain/routecost`); ya se construyó, junto con el Módulo 3 (`domain/fuel`) y una capa de apoyo nueva, `domain/logistics` (radar en vivo), que no estaba prevista en el documento original de 5 módulos. La sección "Lo que sigue" al final ya no aplica tal cual. Para el estado y contrato de API actuales, ve `docs/modulo-2-frontend.md` (routecost) y `docs/modulo-radar-frontend.md` (logistics). El resto de este archivo (arquitectura de auth, bugs encontrados, forma de trabajar, usuarios de prueba) sigue vigente como contexto histórico.

Estoy construyendo el backend de **CargoVigil**, una plataforma SaaS fintech+logtech multi-tenant para empresas de transporte (terrestre/marítimo/aéreo). Repo: `github.com/Hackmty-Billyy/cargovigil-backend`, Go + Fiber + Postgres (contenedor Docker `timescaledb`, imagen `timescale/timescaledb:latest-pg16`). Es un proyecto de hackathon (Hackmty) con otro compañero (estebanc) haciendo commits en paralelo directo a migraciones.

Ya se implementó y **probó contra Postgres real** (no solo compila, se verificó con requests HTTP reales y `docker exec psql`) lo siguiente. Antes de tocar nada, lee el código existente — este resumen es orientación, no reemplaza revisar los archivos reales.

## Arquitectura ya construida

**Estructura de carpetas**: me dieron `config/`, `database/migrations/`, `domain/{fuel,pod,profitability,routecost,treasury}/`, `integrations/{banking,fuelindex,podscanner}/`, `jobs/`, `middleware/`, `server/` como base a seguir. `domain/auth` y `domain/company` se agregaron como desviaciones justificadas (auth y tenancy no estaban en la lista pero son fundacionales). **`domain/routecost` ya está reservado en esa estructura para el Módulo 2** — es donde debería vivir este trabajo.

### Auth (`domain/auth`)
Login + TOTP como segundo factor (compatible Google Authenticator: SHA1/6 dígitos/30s), recovery codes de respaldo. Argon2id para passwords, AES-256-GCM para cifrar el secreto TOTP en BD, JWT access token de vida corta + JWT "mfa-pending" (secreto de firma separado) + refresh token opaco rotable con detección de reuso (si se reusa un token ya rotado, se revocan todas las sesiones). Rate limiting en memoria en `/auth/login`. 4 roles fijos, uno por usuario vía `role_id` FK: `admin`(1), `operations`(2), `finance`(3), `platform_admin`(4).

**No existe auto-registro público** — se quitó `/auth/register`. El JWT de access token lleva `role` y `company_id` como claims.

### Módulo 0 — Identidad/Tenancy/Catálogo (`domain/company`)
Multi-tenant real: `companies`, `users.company_id` (NOT NULL, con retrofit/backfill ya aplicado). Catálogo por empresa: `vehicles`, `routes`, `clients`, `contracts`. Alta de empresas **solo por `platform_admin`** vía `POST /platform/companies` (crea Company + su admin fundador). `POST /company/teammates` (solo admin) invita gente a su propia empresa con rol elegido. CRUD de catálogo: lectura abierta a cualquier rol de la empresa, escritura solo `admin`+`operations`.

### Migraciones de mi compañero (000011-000017) — ya migradas, sin código Go todavía
Construyó el esquema de casi todos los módulos restantes de golpe:
- **`trips`**: entidad central (vehicle_id, route_id, client_id, contract_id?, tracking_code, cargo_type, cargo_weight_tons, status enum `scheduled/in_transit/delayed/completed/cancelled`, departure/estimated/actual arrival dates, agreed_freight_price, currency, fuel_surcharge_amount, **contingency_budget**)
- `bank_accounts`, `invoices`, `expenses`, `cash_alerts` (Módulo 1, ya tiene código Go, ver abajo)
- **`trip_frictions`** (event_type: port_demurrage/customs_delay/traffic_congestion/mechanical_failure/route_deviation/weather_hazard/security_incident/warehouse_detention, duration_hours, cost_impact) — **esto ya ES el `DowntimeEvent` que pide el Módulo 2**
- **`route_risk_profiles`** (historical_risk_score, avg_delay_hours, suggested_contingency_percentage, incident_count) — **esto ya ES el `RouteRiskProfile` que pide el Módulo 2**
- `fuel_indexes` (global, sin company_id) + `trip_fuel_logs` (Módulo 3)
- `pod_documents` + `client_payment_behavior` (Módulo 4)
- `trip_profitability` (Módulo 5)
- `000017`: seed de demo para la empresa "CargoVigil Dev"

**Importante**: `trips`, `trip_frictions` y `route_risk_profiles` YA EXISTEN a nivel de base de datos. El Módulo 2 es construir el paquete Go (`domain/routecost`, siguiendo la carpeta ya reservada) alrededor de este esquema existente — no hay que crear esas tablas de nuevo. Solo falta modelar **`ContingencyFund`**, que NO existe: `trips.contingency_budget` hoy es solo un número estático puesto al crear el viaje, no un fondo calculado/gestionado.

### Módulo 1 — Tesorería Predictiva (`domain/treasury`) — recién terminado
Migración `000018` agregó `invoices.bank_account_id`/`expenses.bank_account_id` (nullable, se llenan al pagar) y la tabla `cash_flow_projections` (company_id, projected_date, projected_balance, único por company+fecha).

Patrón importante de este paquete: **un solo struct `PostgresRepository` implementa 5 interfaces** (BankAccount/Invoice/Expense/CashAlert/CashFlowProjection) con métodos sufijados (`CreateInvoice`, `CreateExpense`, etc.) en vez de nombres genéricos, porque Go no permite dos métodos `Create` con firmas distintas en el mismo tipo — y los pagos necesitan tocar `invoices`/`expenses` Y `bank_accounts` en una sola transacción.

Motor de pronóstico: 60 días, salda base = suma de `bank_accounts` **activas y en MXN únicamente** (bug real que encontré y corregí: mezclar MXN+USD sin conversión daba un número sin sentido — limitación conocida, documentada en el código). Ingresos por `invoices.adjusted_due_date` (ya trae el ajuste por POD), egresos por `expenses.due_date`. Alertas (`cash_alerts`) con severidad `critical/high/medium/low` según qué tanto cae bajo el umbral. Job diario (`jobs/treasury_forecast.go`, corre una vez al iniciar + cada 24h). Endpoints bajo `/treasury/*`, **solo `admin`+`finance`** (operations sin acceso ni de lectura, a diferencia del catálogo).

### `cmd/seed/main.go`
Generador de datos de volumen (no es migración, se corre manual: `go run ./cmd/seed`). Detecta todas las empresas activas automáticamente y genera ~25 viajes por empresa con facturas/gastos/fricciones/combustible/PODs/rentabilidad realistas, más una ráfaga de gastos grandes a propósito para garantizar que el motor de alertas dispare de verdad. Es aditivo (se puede correr varias veces, cada corrida agrega más).

## Bugs reales encontrados y corregidos (para no reintroducirlos)
1. **Gotcha de Fiber**: un `router.Group("", middleware)` (sub-grupo protegido) aplicado sobre el mismo grupo padre "filtra" el middleware a rutas públicas registradas DESPUÉS en ese mismo padre. Las rutas públicas deben registrarse antes de crear el sub-grupo protegido.
2. Config de CORS que alguien agregó mezclaba orígenes explícitos con un `*` suelto en el mismo string — Fiber truena con eso (`panic: [CORS] Invalid origin format`). Se quitó el wildcard.
3. Bug de monedas en el forecast (ver arriba).
4. Listas vacías devolvían `null` en vez de `[]` en JSON — se corrigió inicializando slices como `[]T{}`.
5. En esta máquina de desarrollo, un Postgres nativo de Windows compite con el contenedor Docker por el puerto 5432 (IPv4 vs IPv6) — si hay problemas de conexión/auth rara, revisar `Get-NetTCPConnection -LocalPort 5432` antes de asumir que son credenciales mal puestas.

## Usuarios de prueba (todos `Password123!`)
`platform@cargovigil.test` (platform_admin) · `admin@cargovigil.test` (admin, CargoVigil Dev, **ya tiene TOTP habilitado**) · `admin.mfa@cargovigil.test` (admin, TOTP) · `operaciones@cargovigil.test` (operations) · `finanzas@cargovigil.test` (finance) · `fundador@transportesdemo.test` (admin, Transportes Demo). También existen "Misaemprea" y "La empresa primera", creadas externamente probando `/platform/companies` — no son mías, no las toques sin preguntar.

## Cómo prefiero trabajar (importante)
- Vamos módulo por módulo, en orden. Para cada uno: **primero se discute/diseña, se me pregunta lo que sea una decisión de arquitectura real (no adivinar), y solo hasta que yo diga "procede" se implementa.**
- **Nunca hagas commits de git tú** — yo los hago.
- Respuestas concisas, sin relleno. Avísame de inmediato si algo es crítico del sistema.
- "Mejores prácticas" no es opcional — se espera un análisis real de seguridad/eficiencia antes de fijar un diseño, no la primera idea que funcione.
- **Todo se prueba contra el Postgres real corriendo** (`docker exec timescaledb psql ...` y requests HTTP reales), no basta con que compile. Antes de declarar algo terminado, verifícalo end-to-end.
- El repo tiene a otro compañero empujando cambios en paralelo (migraciones, ediciones de archivos). Antes de asumir el estado del código, corre `git log`/`git status` y relee los archivos relevantes — pueden haber cambiado desde la última vez.

## Lo que sigue: Módulo 2 — Inteligencia de Costes Operativos y Fricciones en Ruta

> Responsabilidad: traducir ineficiencias físicas (tiempos muertos, riesgo de ruta) en impacto financiero.
>
> Entidades de dominio: Trip (viaje), DowntimeEvent (demurrage/detention), RouteRiskProfile, ContingencyFund.
>
> Funciones core:
> - Registro y cálculo de tiempos de espera por viaje (puerto/aduana/CD).
> - Cálculo de "pérdida de oportunidad" por unidad de tiempo inactivo.
> - Asignación automática de colchón de liquidez por viaje según riesgo histórico de la ruta.
>
> Alimenta a: Módulo 1 (ajusta proyección de caja), Módulo 5 (afecta rentabilidad real).
> Fuentes externas: telemetría GPS de flota, datos de clima, histórico de incidentes por ruta.

Puntos que ya sé que hay que resolver en el diseño (no los resuelvas solo, pregúntame):
- `Trip` y `DowntimeEvent` (`trip_frictions`) y `RouteRiskProfile` (`route_risk_profiles`) **ya existen en BD** — el trabajo es exponerlos vía Go (repos/service/handlers en `domain/routecost`), no volver a crear el esquema.
- `ContingencyFund` no existe — hay que decidir si es una tabla nueva (fondo acumulado que se abona/gasta por viaje) o un campo calculado que ajusta `trips.contingency_budget` en el momento de crear el viaje.
- El cálculo de "pérdida de oportunidad" necesita una definición concreta (¿tarifa por hora inactiva? ¿basada en el ingreso que ese vehículo dejó de generar en otro viaje?).
- La integración con Módulo 1 ("ajusta proyección de caja") necesita un mecanismo concreto: probablemente el colchón de contingencia calculado debería generar una `expense` en tesorería para que el forecast lo considere — hay que diseñarlo, no soy yo quien decida solo.
- No hay telemetría GPS/clima real ni la habrá pronto — igual que tesorería, esto va a ser captura manual/simulada por ahora.

Empieza preguntándome sobre estos puntos de diseño antes de escribir código, como hicimos con los módulos anteriores.
