# CargoVigil — Documentación técnica y pitch

Documento centralizado: qué es, cómo se usa de punta a punta, y por qué se construyó con cada pieza de tecnología que trae. Pensado para presentar el proyecto (pitch) y como referencia técnica para el equipo — no reemplaza los docs de detalle por módulo (`docs/project.txt`, `docs/modulo-2-frontend.md`, `docs/modulo-radar-frontend.md`), los complementa.

---

## 1. Resumen ejecutivo

**CargoVigil** es una plataforma SaaS multi-tenant fintech + logtech para empresas de transporte de carga (terrestre, marítimo y aéreo) en México. Conecta dos mundos que casi siempre viven separados en estas empresas: **operación** (viajes, retrasos, combustible) y **finanzas** (flujo de caja, liquidez). Un camión detenido en aduana dentro de CargoVigil no es solo un evento en un mapa — es un gasto real que consume un colchón de contingencia y que, si se acaba, mueve la proyección de caja de la empresa a 60 días, en vivo.

Ese circuito cerrado (operación → finanzas → operación) es el diferenciador del producto. El resto de este documento explica cómo se usa y por qué se construyó como se construyó.

---

## 2. Flujo de uso de punta a punta

Así es como una empresa cliente experimenta la plataforma, en orden:

1. **Alta de la empresa.** Un miembro del equipo de CargoVigil (`platform_admin`) da de alta a la empresa cliente y a su administrador fundador desde el panel de plataforma (`POST /platform/companies`). Cada empresa es un tenant aislado — ningún dato cruza entre empresas.
2. **Primer login y consentimiento.** El administrador fundador entra por primera vez, activa (opcionalmente) 2FA con TOTP, y — antes de ver cualquier pantalla — acepta el Aviso de Privacidad, incluyendo el consentimiento expreso para el tratamiento de sus datos financieros (cuentas bancarias, saldos).
3. **Catálogo base.** El admin (o quien invite con rol `operations`) carga el catálogo operativo: vehículos (camión/barco/avión), rutas con origen-destino, clientes y contratos. Este catálogo alimenta a todos los módulos siguientes.
4. **Cuentas bancarias y tesorería.** El rol `finance` (o `admin`) registra las cuentas bancarias de la empresa con su saldo actual y el mínimo requerido — el punto de partida del pronóstico de caja.
5. **Se programa un viaje.** Desde el Radar en Vivo o desde el módulo de Fricciones y Costos, se agenda un viaje: vehículo, ruta, cliente, flete acordado. En el instante en que se crea, el backend calcula automáticamente un **colchón de contingencia** según el riesgo histórico de esa ruta (nunca se manda desde el cliente) y ese colchón aparece como un gasto pendiente en Tesorería — la proyección de caja a 60 días ya lo refleja.
6. **El viaje avanza.** En producción esto vendría de telemetría GPS real; hoy lo simula el botón "Avanzar tiempo" del radar. El viaje se mueve por el mapa, y con cierta probabilidad ocurre un incidente (aduana, clima, falla mecánica...).
7. **Ocurre una fricción.** El incidente se registra como una fricción real del Módulo 2: horas muertas, costo directo (lo que se le paga a un tercero) y pérdida de oportunidad (lo que el activo dejó de producir) — dos cifras distintas, nunca sumadas como una sola.
8. **El colchón absorbe el golpe.** Al cerrarse la fricción, su costo se cobra *contra* el colchón de ese viaje — la reserva se encoge por el mismo monto en vez de sumarse encima. La salida total de caja proyectada no cambia; solo el excedente sobre el colchón sería dinero nuevo saliendo de la proyección.
9. **El viaje llega.** Se libera el remanente del colchón a caja y se recalcula el score de riesgo de esa ruta — el siguiente viaje que la use apartará más o menos colchón según lo que acaba de pasar.
10. **Combustible y margen.** En paralelo, el Módulo 3 compara el precio actual del índice de combustible contra el precio con el que se cotizaron las rutas; si la variación pasa el umbral que la empresa puede absorber, recomienda un % de recargo (fuel surcharge), y permite simular "¿qué pasa con mi margen si el diésel sube 15%?" antes de que pase de verdad.
11. **Decisión antes de comprometerse.** Antes de agendar cualquier viaje, el programador ("¿Conviene este viaje?") combina los tres módulos — combustible, riesgo de ruta, colchón sugerido — y devuelve margen neto proyectado y una recomendación en texto, en la moneda en la que se está cotizando.
12. **Alertas.** Si en cualquier punto la proyección de caja cae debajo del mínimo requerido, se dispara una alerta de liquidez con severidad graduada (`low` → `critical`), visible en el dashboard de Tesorería.

Todo el flujo queda auditado: cada colchón guarda el score de riesgo, el % aplicado y el tipo de cambio con el que se calculó, así que un cambio de tipo de cambio mañana nunca reescribe el histórico de hoy.

---

## 3. Arquitectura general

```
┌─────────────────────────────┐        ┌──────────────────────────────────┐
│  Frontend (React + Vite)    │  HTTPS │  Backend (Go + Fiber)             │
│  Dashboard, Radar, Tesorería│◄──────►│  domain/{auth,company,treasury,   │
│  Módulo 2, Módulo 3         │  JWT   │   routecost,fuel,logistics}       │
└─────────────────────────────┘        └───────────────┬──────────────────┘
                                                         │ Bun / pgx
                                                         ▼
                                        ┌──────────────────────────────────┐
                                        │  PostgreSQL + TimescaleDB         │
                                        │  Tablas relacionales + hypertables│
                                        │  + continuous aggregates          │
                                        └──────────────────────────────────┘
```

Tres repos separados (`cargovigil-frontend`, `cargovigil-backend`, `cargovigil-db`), cada uno con su propio ciclo de vida, y tres entornos de base de datos (`local`, `dev`, `prod`) con aislamiento de red y límites de recursos crecientes.

---

## 4. Por qué TimescaleDB (Tiger Data)

TimescaleDB es un producto de **Tiger Data** (antes Timescale Inc.) — una extensión de PostgreSQL, no una base de datos aparte. Esa última parte es la razón real de la elección:

- **Datos genuinamente de series de tiempo, en el mismo motor relacional.** `fuel_indexes` (precio de combustible en el tiempo), `trip_fuel_logs` (cargas de combustible por viaje), `trip_frictions` (incidentes en ruta) y `cash_flow_projections` (la proyección de caja recalculada cada día) son, todos, streams de eventos con marca de tiempo que crecen sin parar. Modelarlos en tablas relacionales normales funciona al principio, pero degrada: los índices se hacen enormes, las consultas de rango de fecha se vuelven lentas, y nada las poda solas.
- **Hypertables particionan automáticamente por tiempo.** `create_hypertable('trip_fuel_logs', 'purchased_at', chunk_time_interval => '30 days')` — cada consulta que filtra por fecha (que es casi todas) solo toca los chunks relevantes, no la tabla completa. Esto se decidió en la migración `000019`, después de que las primeras cuatro tablas de este tipo mostraron cuánto iban a crecer con el uso real.
- **Continuous aggregates evitan recalcular sobre el histórico crudo.** `fuel_price_weekly` es una vista materializada que Timescale refresca sola cada día — el Módulo 3 lee una tendencia semanal ya calculada en vez de escanear meses de precios cada vez que alguien abre esa pantalla.
- **Políticas de compresión y retención declarativas.** `trip_fuel_logs` se comprime automáticamente después de 30 días (`add_compression_policy`) porque es una bitácora de solo-inserción que nadie actualiza; `cash_flow_projections` se poda a los 90 días (`add_retention_policy`) porque nadie necesita el pronóstico que se hizo hace 4 meses para una fecha que ya pasó. Sin esto, alguien tendría que escribir un cron para hacer lo mismo a mano.
- **Cero costo de integración.** Como es una extensión de Postgres, **todo el resto del stack la ve como Postgres normal**: el mismo driver (`pgx`), el mismo ORM (Bun), las mismas migraciones (`golang-migrate`), el mismo `docker-compose`. No hay un segundo sistema que aprender, desplegar o respaldar por separado — la complejidad de "series de tiempo" se paga una sola vez, en las migraciones, no en cada capa de la aplicación.

En corto: se necesitaban las garantías de una base relacional (transacciones, joins, integridad referencial entre `trips`, `routes`, `clients`) **y** el rendimiento de series de tiempo para los datos que de verdad se comportan así — TimescaleDB da ambas cosas sin duplicar infraestructura.

---

## 5. Por qué se sube a la nube

El desarrollo corre 100% local (Docker Compose, `cargovigil-db`), pero el producto está pensado para vivir en la nube desde el diseño, no como un paso posterior:

- **Es un SaaS multi-tenant real, no una herramienta interna.** El valor del producto depende de que empresas de transporte externas —que no tienen ni quieren tener su propio servidor— puedan entrar desde cualquier lugar con un navegador. Eso solo es posible con el backend y la base de datos accesibles por internet, con dominio propio y TLS (`cargovigil.tech`, `apidev.cargovigil.tech`, ya configurados en CORS).
- **Separación real de entornos.** `cargovigil-db` ya define tres perfiles de despliegue (`local`, `dev`, `prod`) con `docker-compose.{local,dev,prod}.yml`: producción corre **sin puertos expuestos al host** (solo accesible por la red interna de Docker desde el backend), con límite de 2 GB de RAM y rotación de logs — separación que no tiene sentido mantener si todo va a vivir en una sola laptop.
- **Disponibilidad fuera del horario de quien lo programó.** Un dashboard de flujo de caja que solo funciona cuando la laptop del desarrollador está prendida no sirve para un cliente real que necesita ver su alerta de liquidez un domingo en la noche.
- **Los jobs automáticos necesitan un proceso siempre vivo.** `treasury_forecast`, `route_risk` y `fuel_surcharge_recalc` corren cada 24 h — eso exige un proceso persistente, no una ejecución manual cada vez que alguien se acuerda.
- **Cumplimiento ya contemplado, no una sorpresa después.** Subir a la nube implica una transferencia de datos que la LFPDPPP exige declarar — por eso el Aviso de Privacidad (`src/components/legal/`) ya incluye la sección de transferencias internacionales antes de que exista un solo cliente real en producción, no como un parche posterior.

---

## 6. Por qué cada tecnología

### Backend

| Tecnología | Por qué esta y no otra |
|---|---|
| **Go** | Tipado estático + compilación a binario único simplifica el despliegue (sin runtime que instalar en el servidor), buen rendimiento en I/O concurrente (cada request de la API es una goroutine barata), y un ecosistema maduro para lo que este producto necesita: drivers de Postgres, JWT, criptografía de la librería estándar. |
| **Fiber v2** | Framework HTTP inspirado en Express sobre `fasthttp` — sintaxis conocida (`router.Get`, middlewares) con mejor rendimiento que `net/http` puro, y con lo que un equipo armando esto en un hackathon necesita: grupos de rutas, middlewares por grupo, manejo de JSON de serie. |
| **PostgreSQL + TimescaleDB** | Ver sección 4. Relacional cuando se necesita integridad (multi-tenant, FKs), series de tiempo cuando los datos crecen así. |
| **Bun (ORM)** | Se adoptó a mitad de proyecto para eliminar el SQL escrito a mano en `auth`, `company`, `treasury` y `fuel` sin perder el control fino que Postgres necesita. Se descartó GORM/Ent porque ambos quieren ser dueños del esquema (auto-migración), lo cual choca directo con las hypertables de PK compuesta que ya existían — Bun se usa puramente como capa de queries/mapeo, el esquema lo sigue gobernando `golang-migrate` en solitario. `domain/routecost` se quedó en `pgx` crudo porque se construyó en paralelo antes de esa decisión; convive sin problema porque ambos hablan al mismo pool de conexiones. |
| **golang-migrate** | Migraciones versionadas y embebidas en el binario (`go:embed`) — el esquema viaja con el código, no depende de que alguien corra un script aparte al desplegar. |
| **JWT (access token de vida corta) + refresh token opaco rotable** | Estándar para APIs sin estado; el refresh token opaco (no JWT) permite revocarlo en base de datos, y la detección de reuso (si se reusa un token ya rotado, se cierran todas las sesiones) es la mitigación estándar contra robo de refresh tokens. |
| **TOTP (RFC 6238), compatible Google Authenticator** | Segundo factor sin depender de SMS (que tiene costo y es vulnerable a SIM-swapping) ni de un proveedor externo — el usuario ya trae una app compatible en su teléfono. |
| **Argon2id** | Ganador de la Password Hashing Competition y recomendación actual de OWASP para hashear contraseñas — resistente a ataques por GPU/ASIC a diferencia de bcrypt en cargas de trabajo modernas. |
| **AES-256-GCM** | Cifra el secreto TOTP en reposo en la base de datos — si la base se filtra, el secreto de 2FA de nadie queda expuesto en texto plano. |
| **Rate limiting en `/auth/login`** | Mitigación básica contra fuerza bruta sobre credenciales, sin necesidad de infraestructura externa (Redis, etc.) para el volumen de un producto en esta etapa. |

### Frontend

| Tecnología | Por qué esta y no otra |
|---|---|
| **React 19** | El equipo ya lo conocía, y el ecosistema de componentes (mapas, iconos, formularios) es el más grande disponible para construir rápido en un hackathon sin sacrificar mantenibilidad. |
| **TypeScript** | Mismo argumento que Go en el backend: tipado estático de punta a punta. Los contratos de API (`types/routecost.ts`, `types/logistics.ts`) documentan la forma real de las respuestas y truenan en compilación si el backend cambia algo sin avisar. |
| **Vite** | Arranque y hot-reload casi instantáneos comparado con toolchains más pesados — importa en un hackathon donde el ciclo iterar-ver-ajustar se repite cientos de veces. |
| **Tailwind CSS v4** | Utilidades en el markup en vez de archivos CSS separados — permite iterar el diseño de una pantalla completa sin saltar entre archivos, con consistencia de espaciado/color por defecto. |
| **lucide-react** | Set de iconos consistente y liviano (SVG tree-shakeable), evita mezclar tres librerías de iconos distintas con estilos que no combinan. |
| **react-leaflet / Leaflet** | Mapa interactivo para el Radar en Vivo — Leaflet es liviano, de código abierto, y no depende de una cuenta/API key de pago como Google Maps para lo que este proyecto necesita (marcadores, líneas de ruta, popups). |

### Infraestructura

| Tecnología | Por qué esta y no otra |
|---|---|
| **Docker / Docker Compose** | Mismo entorno en la laptop de cada desarrollador y en el servidor — "en mi máquina sí funciona" deja de ser un problema porque la imagen (`timescale/timescaledb:latest-pg16`) es idéntica en los tres perfiles (`local`/`dev`/`prod`). También es el requisito de facto para correr TimescaleDB sin instalar Postgres a mano. |
| **Repos separados (frontend / backend / db)** | Cada uno se versiona, despliega y escala por su cuenta — el backend no necesita reconstruirse porque alguien cambió un color en el frontend, y la base de datos tiene su propio ciclo de migraciones independiente del código de la aplicación. |

---

## 7. Cumplimiento legal (resumen)

Investigado contra la legislación mexicana vigente antes de pensar en lanzar a mercado real (detalle completo en la conversación de diseño, resumen aquí):

- **LFPDPPP (2025):** aplica de lleno — se trata correo, contraseña (cifrada) y, sobre todo, datos patrimoniales/financieros (cuentas bancarias, saldos), que exigen **consentimiento expreso**. Ya implementado: Aviso de Privacidad público (`/privacidad`) + gate de consentimiento con dos casillas separadas antes de dar acceso al dashboard.
- **Ley Fintech:** no aplica mientras la plataforma no mueva ni custodie dinero de terceros (hoy solo proyecta/visualiza) — el riesgo aparece únicamente si se integra Open Banking real (cuya infraestructura regulatoria en México, para datos transaccionales, sigue incompleta) o si se agrega algún producto de crédito/factoring.
- **Ley Antilavado (LFPIORPI) y Ley de Sociedades de Información Crediticia:** no aplican hoy; ambas se activarían solo si el producto evoluciona hacia prestar dinero o compartir historial de pago de clientes *entre* distintas empresas tenant — límites a tener presentes en el roadmap de los Módulos 4/5, no bloqueos actuales.

---

## 8. Estado actual de los módulos

| Módulo | Backend | Frontend | Prefijo API |
|---|---|---|---|
| 0 — Identidad y catálogo | ✅ | ✅ | `/auth`, `/company`, `/platform` |
| 1 — Tesorería predictiva | ✅ | ✅ | `/treasury` |
| 2 — Fricciones y coste operativo | ✅ | ✅ | `/routecost` |
| 3 — Combustible | ✅ | ✅ | `/fuel` |
| Radar en vivo (capa de apoyo) | ✅ | ✅ | `/logistics` |
| 4 — POD y cobranza | ❌ solo tablas | ❌ | — |
| 5 — Rentabilidad por viaje | ❌ solo tablas | ❌ | — |

Sin fuentes externas reales integradas todavía (Open Banking, telemetría GPS, feed de combustible) — todo captura manual o simulado, documentado así en el código a propósito, no es un descuido.
