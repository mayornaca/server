// i18n bootstrap. Fase 6 plan QA 2026-05-17 — centraliza strings que se
// duplicaban a través de componentes ("Error cargando X" se repetía 8 veces).
// El alcance actual es deliberadamente MÍNIMO: solo los strings duplicados
// detectados en el audit; el resto del UI sigue con strings en español
// inline porque NO hay requisito de localización multi-idioma hoy.
//
// Si en el futuro se agrega inglés u otro idioma, este bootstrap ya está
// listo para `addResourceBundle("en", "translation", enJSON)`.
import i18n from "i18next"
import { initReactI18next } from "react-i18next"
import es from "./es.json"

i18n.use(initReactI18next).init({
  resources: { es: { translation: es } },
  lng: "es",
  fallbackLng: "es",
  interpolation: { escapeValue: false }, // react ya escapa por default
})

export default i18n
