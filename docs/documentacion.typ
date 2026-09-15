#set page(
  paper: "a4",
  margin: (top: 2.5cm, bottom: 2.5cm, left: 3cm, right: 3cm),
  numbering: (current, total) => {
    if current > 1 { str(current - 1) }
  }
)
#set text(font: "Linux Libertine", size: 11pt, lang: "es")
#set par(justify: true, leading: 0.65em)

// --- PORTADA ---
#align(center + horizon)[
  #image("puj-logo.png", width: 30%)
  #v(1cm)
  
  #text(size: 14pt, weight: "bold")[PONTIFICIA UNIVERSIDAD JAVERIANA] \
  #text(size: 11pt, style: "italic")[Facultad de Ingeniería --- Introducción a Sistemas Distribuidos]
  
  #v(2cm)
  #text(size: 22pt, weight: "bold")[
    Diseño del Sistema Reservas de Boletos
  ]

  #v(3cm)
  
  #grid(
    columns: (1fr),
    gutter: 1.2em,
    align(center)[
      *Integrantes:* \
      Elier David Ibarra Medina \
      Samuel Adrian Montaña Linares \
    ]
  )
  
  #v(2cm)
#text(size: 12pt, weight: "bold")[
    Profesor: Rafael Vicente Paez Mendez
  ]
]

#pagebreak() 
