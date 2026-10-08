#set page(
  paper: "a4",
  margin: (top: 2.5cm, bottom: 2.2cm, left: 2.2cm, right: 2.2cm),
  numbering: (current, total) => {
    if current > 1 { str(current - 1) }
  }
)
#set text(font: "Linux Libertine", size: 10pt, lang: "es")
#set par(justify: true, leading: 0.6em, spacing: 0.85em)
#set heading(numbering: "1.1")
#show heading.where(level: 1): set text(size: 13pt)
#show heading.where(level: 2): set text(size: 11pt)
#show heading: set block(above: 1.1em, below: 0.6em)
#set table(stroke: 0.4pt, inset: (x: 4pt, y: 3pt))
#show table: set text(size: 8.5pt)
#show figure.caption: set text(size: 8.5pt)
#set figure(gap: 0.5em)
#show figure: set block(above: 1em, below: 1em)

// --- PORTADA ---
#align(center + horizon)[
  #image("/images/puj-logo.png", width: 30%)
  #v(1cm)

  #text(size: 14pt, weight: "bold")[PONTIFICIA UNIVERSIDAD JAVERIANA] \
  #text(size: 11pt, style: "italic")[Facultad de Ingeniería --- Introducción a Sistemas Distribuidos]

  #v(2cm)
  #text(size: 22pt, weight: "bold")[
    Sistema Reservas de Boletos
  ]
  #v(0.4cm)
  #text(size: 13pt)[Primera entrega --- Arquitectura base]

  #v(3cm)

  #grid(
    columns: (1fr),
    gutter: 1.2em,
    align(center)[
      *Integrantes:* \
      Elier David Ibarra Medina \
      Samuel Adrian Montaña Linares \
    ],
  )

  #v(2cm)
  #text(size: 12pt, weight: "bold")[
    Profesor: Rafael Vicente Paez Mendez
  ]
]

#pagebreak()

= Introducción

El Sistema de Reserva de Boletos es una aplicación distribuida que permite a los estudiantes de la universidad consultar los eventos culturales y deportivos de un mes, reservar puestos y, más adelante, modificar o cancelar sus reservas. Se organiza en tres capas (consumidores, lógica de negocio y almacenamiento), cada una en una máquina distinta, que se comunican con ZeroMQ combinando patrones síncronos (consulta y reserva) y asíncronos (cancelación).

Esta primera entrega cubre la arquitectura base: la consulta y la reserva funcionan de extremo a extremo (cliente → Gestor de Transacciones → RMC → Persistencia), sin tolerancia a fallas. El informe presenta los cuatro modelos del sistema, cada uno con los diagramas de diseño que le corresponden, y define el protocolo de pruebas, la forma de obtener las métricas y el mecanismo de generación de carga. La cancelación, la modificación, la escalabilidad y la tolerancia a fallas se diseñan aquí pero se implementan en la segunda entrega.

= Modelos del Sistema

== Modelo Físico

#figure(
  image("images/despliegue_diagrama.png", width: 100%),
  caption: [Diagrama de despliegue: nodos, componentes, puertos y comunicación entre capas.],
) <fig-despliegue>

#figure(
  image("images/componentes_diagrama.png", width: 100%),
  caption: [Diagrama de componentes: servicios del sistema y sus interfaces de comunicación.],
) <fig-componentes>

#figure(
  image("images/clases_diagrama.png", width: 100%),
  caption: [Diagrama de clases: modelo de datos, mensajes del protocolo y almacén de la persistencia.],
) <fig-clases>

*Modelo físico.* El sistema se despliega en tres dispositivos (@fig-despliegue), uno por capa. Como no están en la misma red física, se conectan mediante *Tailscale*, una red privada virtual que les asigna direcciones IP fijas (100.x.y.z) sin importar desde dónde se conecten.

#table(
  columns: (auto, 1.6fr, 1.2fr, 1fr, 3fr),
  align: left,
  [*Nodo*], [*Dispositivo*], [*Sistema operativo*], [*RAM / disco*], [*Rol*],
  [1], [Lenovo ThinkPad E16 Gen2 (Samuel Montaña)], [Linux Mint 22.3], [16 GB / 512 GB], [Capa de consumidores: procesos cliente que generan las solicitudes de consulta y reserva.],
  [2], [Dell XPS 13 9310 (Elier Ibarra)], [Linux Mint 22.1], [15 GB / 229 GB], [Capa de negocio: Gestor de Transacciones, RMC y (Entrega 2) Cancelaciones y Notificaciones.],
  [3], [Google Pixel 9a], [Android 17], [8 GB / 128 GB], [Capa de almacenamiento: Servicio de Persistencia, más un cliente adicional que genera carga y mide sus tiempos de respuesta.],
)

*Modelo arquitectónico.* Es una arquitectura cliente-servidor de tres capas (@fig-componentes). Los *procesos cliente* envían solicitudes al *Gestor de Transacciones* (GT), la puerta de entrada, que las enruta por el canal síncrono hacia el *RMC* (reserva, modificación y consulta); el RMC consulta y actualiza los datos a través del *Servicio de Persistencia*. El servicio de Cancelaciones y su canal asíncrono forman parte del diseño pero se implementan en la Entrega 2. Cada servicio tiene un puerto fijo para que la comunicación sea predecible:

#table(
  columns: (3fr, 0.8fr, 0.8fr, 2.6fr),
  align: left,
  [*Componente*], [*Nodo*], [*Puerto*], [*Socket ZeroMQ*],
  [Gestor de Transacciones, canal síncrono], [2], [9900], [REP (clientes se conectan con REQ)],
  [Gestor de Transacciones, canal asíncrono], [2], [9901], [ROUTER (Entrega 2)],
  [Servicio RMC], [2], [9902], [REP (el GT se conecta con REQ)],
  [Cancelaciones y Notificaciones], [2], [9903], [PULL / PUB (Entrega 2)],
  [Servicio de Persistencia], [3], [9904], [REP (el RMC se conecta con REQ)],
  [Procesos cliente], [1 y 3], [---], [REQ: solo hacen `connect`, no exponen puerto],
)

El sistema está escrito en *Go* (versión 1.27.1) con la librería `go-zeromq/zmq4` (versión 0.17.0), escogidos por tres razones: las _goroutines_ permiten simular muchos clientes concurrentes con poco costo; al ser compilado, sus tiempos de respuesta son consistentes para las métricas del proyecto; y la librería es Go puro, sin dependencias de C, de modo que cada servicio es un binario único que se copia a cada nodo (incluido el celular, compilando para ARM64). Los mensajes se serializan en JSON.

*Datos.* El modelo de datos (@fig-clases) sigue el enunciado: 50 eventos en un mismo mes, 20 con dos ocurrencias y 30 con una, y 100 asientos por ocurrencia; los asientos se controlan por ocurrencia (libres + ocupados = totales). La persistencia mantiene los datos en memoria protegidos con un _mutex_ y los guarda en un archivo JSON (_snapshot_), por lo que sobreviven a un reinicio. Cada respuesta lleva un código: `OK`, `RECHAZADA` (regla de negocio, como falta de cupo) o `ERROR` (falla del sistema).

== Modelo de Interacción

#figure(
  image("images/secuencia_reserva_diagrama.png", width: 88%),
  caption: [Secuencia de una reserva (síncrona, implementada): REQ/REP en los puertos 9900, 9902 y 9904.],
) <fig-sec-reserva>

#figure(
  image("images/secuencia_cancelacion_diagrama.png", width: 88%),
  caption: [Secuencia de una cancelación (asíncrona, diseño): DEALER/ROUTER :9901, PUSH/PULL :9903, REQ :9904 y PUB/SUB.],
) <fig-sec-cancelacion>

Las interacciones se dividen según cómo espera la respuesta el cliente:

#table(
  columns: (3fr, 1.2fr, 2fr),
  align: left,
  [*Interacción*], [*Tipo*], [*Patrón ZeroMQ*],
  [Cliente → GT (consulta, reserva)], [Síncrona], [REQ/REP],
  [GT → RMC], [Síncrona], [REQ/REP],
  [RMC → Persistencia], [Síncrona], [REQ/REP],
  [Cliente → GT (cancelación) y confirmación inmediata], [Asíncrona (Entrega 2)], [DEALER/ROUTER],
  [GT → Cancelaciones], [Asíncrona (Entrega 2)], [PUSH/PULL],
  [Cancelaciones → Cliente (notificación)], [Asíncrona (Entrega 2)], [PUB/SUB],
)

*Operaciones síncronas.* El cliente envía la solicitud y espera la respuesta antes de continuar. En una reserva (@fig-sec-reserva) el RMC primero consulta a la persistencia la disponibilidad en tiempo real y, si hay cupo, confirma la reserva; la persistencia vuelve a validar el cupo y descuenta los asientos dentro de una sola sección crítica, por lo que dos clientes que compiten por los últimos puestos nunca producen sobreventa. La consulta sigue el mismo camino con un solo paso hacia la persistencia. Cada solicitud lleva un identificador único (`id_solicitud`) para trazarla en los registros de todos los servicios.

*Operaciones asíncronas.* En la cancelación (@fig-sec-cancelacion) el GT confirma la recepción de inmediato y el cliente queda libre; cuando la persistencia libera los asientos, el servicio de Cancelaciones publica una notificación que el cliente recibe por separado.

*Supuestos.* El sistema es asíncrono en el sentido de los sistemas distribuidos: no hay cotas garantizadas de latencia entre nodos, porque el tráfico viaja por Tailscale. REQ/REP impone alternancia estricta (una solicitud a la vez por socket), por lo que el GT, el RMC y la persistencia atienden una solicitud a la vez; esta limitación es deliberada en la arquitectura base y es el punto que la arquitectura escalable debe resolver.

== Modelo de Fallos

#figure(
  image("images/fallos_diagrama.png", width: 100%),
  caption: [Diagrama del modelo de fallos: puntos de falla y tiempos límite anidados.],
) <fig-fallos>

Se adopta el modelo de fallas por caída (_crash-stop_): un componente que falla deja de responder, pero no envía respuestas incorrectas ni corruptas. Los puntos de falla considerados son el Gestor de Transacciones y el RMC (@fig-fallos), responsables del flujo de consulta y reserva; la persistencia se trata como dependencia. También se contemplan fallas de omisión, es decir, mensajes perdidos por una interrupción de la conexión Tailscale, que desde fuera no se distinguen de una caída. No se consideran fallas arbitrarias (bizantinas).

La detección se basa en tiempos límite (_timeouts_) anidados: el cliente espera hasta 15 s al GT, el GT espera hasta 10 s al RMC y el RMC espera hasta 5 s a la persistencia. Como cada nivel espera menos que el anterior, una falla aguas abajo llega al cliente como una respuesta `ERROR` controlada y no como un bloqueo. Tras un _timeout_ el solicitante cierra su socket REQ y crea uno nuevo, pues un socket REQ queda bloqueado esperando una respuesta que ya no llegará. Es un detector imperfecto (no distingue una caída de una demora), con el compromiso habitual entre detectar rápido y evitar falsos positivos. En esta entrega solo se detecta y registra la falla; la recuperación se aborda en la Entrega 2.

== Modelo de Seguridad

#figure(
  image("images/seguridad_diagrama.png", width: 100%),
  caption: [Diagrama del modelo de seguridad: perímetro de confianza y riesgos aceptados.],
) <fig-seguridad>

El perímetro de confianza es la red Tailscale (@fig-seguridad): los tres nodos se comunican dentro de un túnel cifrado con WireGuard, y un dispositivo que no pertenece a la red no puede alcanzar los servicios. ZeroMQ no cifra por sí mismo, así que la *confidencialidad* y la *integridad* en tránsito dependen del túnel. No hay *autenticación* a nivel de aplicación: cualquier proceso dentro de la red Tailscale del equipo podría enviar solicitudes al GT. Como control básico, cada servicio valida el formato de los mensajes y las reglas de negocio (mes, evento, ocurrencia y cantidad) y responde con un error controlado ante datos inválidos. Estos riesgos se aceptan porque el entorno de pruebas es controlado y solo el equipo tiene acceso a la red; como mejora se propone el mecanismo CURVE de ZeroMQ para autenticar clientes y cifrar a nivel de aplicación.


= Arquitectura escalable y tolerante a fallas

Para la segunda entrega se propone: (i) _escalabilidad_: reemplazar REQ/REP por ROUTER/DEALER en el GT y colocar varias réplicas del RMC detrás de un balanceador, de modo que se atiendan solicitudes concurrentes, y replicar la base de datos con un esquema primario--réplica, enviando las consultas a las réplicas y las escrituras al primario; (ii) _tolerancia a fallas_: réplicas pasivas del GT y del RMC vigiladas con latidos (_heartbeats_), de modo que cuando se detecte una caída la réplica asuma el puerto del proceso caído. Para que las operaciones continúen en el punto en que se dejaron, el `id_solicitud` permite reintentos idempotentes y el estado de las operaciones en curso se registrará en la capa de persistencia; (iii) completar el canal asíncrono de cancelación (DEALER/ROUTER, PUSH/PULL y PUB/SUB).

= Protocolo de Pruebas

#table(
  columns: (1.1fr, 2.6fr, 2.4fr, 2.4fr),
  align: left,
  [*Tipo*], [*Qué valida*], [*Cómo*], [*Criterio de éxito*],
  [Unitarias], [Almacén de datos (carga inicial, consulta por mes, reserva, rechazos) y cálculo de métricas.], [`go test ./internal/...`], [Pasan todas; en toda ocurrencia libres + ocupados = 100.],
  [Integración], [Flujo cliente → GT → RMC → Persistencia, y persistencia en disco.], [`go test ./tests/` levanta los tres servicios en localhost.], [La consulta devuelve 50 eventos; la reserva descuenta asientos y queda en el _snapshot_.],
  [Funcionales], [Consulta con y sin eventos, mes inválido, reserva válida, sin cupo, evento u ocurrencia inexistente, cantidad inválida, operación no soportada.], [Cliente interactivo (`-interactivo`) sobre los tres nodos.], [Cada caso devuelve `OK`, `RECHAZADA` o `ERROR` esperado y la BD queda como se espera.],
  [Concurrencia], [Varios clientes reservando la misma ocurrencia.], [Prueba automática con 8 clientes y ejecución con 8 clientes en los nodos.], [Nunca se venden más de 100 asientos.],
  [Despliegue y red], [Conectividad entre los tres nodos y arranque en el orden indicado.], [`tailscale ping` y ejecución real en los tres dispositivos.], [Todos los puertos son alcanzables y el flujo completo funciona.],
  [Rendimiento], [Escenarios de 3, 5 y 8 clientes (Tabla 1).], [Generador de carga, 2--3 min por escenario, 3 repeticiones y BD reiniciada.], [≥ 20 solicitudes/min por cliente y resultados repetibles.],
  [Robustez], [Detección de fallas sin bloqueo.], [Detener el RMC o la persistencia durante la carga.], [El cliente recibe `ERROR` en ≤ 15 s y el sistema sigue atendiendo.],
)

= Obtención de Métricas de Desempeño

Las métricas se obtienen instrumentando el código, sin herramientas externas de monitoreo, usando solo la biblioteca estándar de Go (`time`, `sync/atomic`, `encoding/csv`) y de Python (`statistics`).

#table(
  columns: (2.6fr, 1.6fr, 4fr),
  align: left,
  [*Métrica (Tabla 1)*], [*Dónde se mide*], [*Cómo*],
  [Tiempo promedio de respuesta y desviación estándar, de consultas y de reservas], [Cliente (extremo a extremo)], [Se toma `time.Now()` antes de enviar y `time.Since()` al recibir cada respuesta; cada solicitud se guarda en un CSV. La media y la desviación estándar muestral se calculan con el algoritmo de Welford. Las solicitudes con `ERROR` se cuentan aparte.],
  [Transacciones de consulta y de reserva procesadas por minuto (TPM)], [Gestor de Transacciones], [Contadores por operación; TPM = transacciones procesadas / minutos entre la primera y la última. Se imprimen cada 15 s y al cerrar, y se guardan en `metricas_gt.csv`.],
  [Estado de la base de datos], [Persistencia], [Resumen (eventos, asientos libres y ocupados, reservas) al iniciar, periódicamente y al cerrar, más el _snapshot_ JSON.],
)

= Generación de Carga

La carga la produce un generador propio escrito en Go (`cmd/cliente`). No se usaron Locust ni JMeter porque están orientados a HTTP y los servicios hablan ZeroMQ; reutilizar el mismo protocolo y la misma librería evita traducciones que alterarían las mediciones. Cada cliente simulado es una _goroutine_ con su propio socket REQ que envía solicitudes a un ritmo fijo, con un desfase inicial aleatorio. Antes de reservar, el cliente consulta el catálogo para elegir ocurrencias existentes. Son parametrizables el número de clientes (`-clientes`), cuántos solo consultan (`-solo-consulta`), las solicitudes por minuto por cliente (`-rpm`, mínimo 20), el porcentaje de consultas de los clientes mixtos (`-pct-consulta`), la duración (`-duracion`), el mes y el máximo de asientos por reserva.

#table(
  columns: (1fr, 2.4fr, 3.4fr),
  align: left,
  [*Escenario*], [*Perfil*], [*Parámetros*],
  [3 clientes], [2 solo consulta + 1 consulta y reserva], [`-clientes 3 -solo-consulta 2 -pct-consulta 50`],
  [5 clientes], [3 solo consulta + 2 reserva], [`-clientes 5 -solo-consulta 3 -pct-consulta 0`],
  [8 clientes], [4 solo consulta + 4 reserva], [`-clientes 8 -solo-consulta 4 -pct-consulta 0`],
)

Los clientes se reparten entre el nodo 1 y el nodo 3, que ejecutan sus comandos al mismo tiempo. Como la modificación se implementa en la Entrega 2, los perfiles "reserva y modificación" usan solo reservas en esta entrega.

// TODO: cómo los clientes generan las solicitudes parametrizables
