export const contacts = Object.freeze({
    commercial: Object.freeze({
        number: '5549988502101',
        display: '+55 49 98850-2101',
    }),
    secondaryWhatsApp: Object.freeze({
        number: '5549999862222',
        display: '+55 49 99986-2222',
    }),
    travelAi: Object.freeze({
        number: '5549991600330',
        display: '+55 49 99160-0330',
    }),
    legacyVoice: Object.freeze({
        number: '+554932466666',
        display: '(49) 3246-6666',
    }),
})

const travelAiDestinations = new Set([
    'maranhao',
    'sc-balneario',
    'sc-beto-carrero',
    'sc-combo',
])

export function createWhatsAppUrl(contact, message = '') {
    const baseUrl = `https://wa.me/${contact.number.replace(/^\+/, '')}`
    return message ? `${baseUrl}?text=${encodeURIComponent(message)}` : baseUrl
}

export function createTelUrl(contact) {
    const number = contact.number.startsWith('+') ? contact.number : `+${contact.number}`
    return `tel:${number}`
}

export function resolveBookingContact(destination) {
    return travelAiDestinations.has(destination) ? contacts.travelAi : contacts.commercial
}
