import { Button, Input, Select, SelectItem, Textarea } from '@heroui/react'
import { motion as Motion } from 'framer-motion'
import { Calendar, CheckCircle, Mail, MapPin, MessageCircle, Phone, User, Users } from 'lucide-react'
import { useState } from 'react'
import { createWhatsAppUrl, resolveBookingContact } from '../data/contacts'

const destinations = [
    { key: 'maranhao', label: 'Lençóis Maranhenses (6-7 dias)' },
    { key: 'sc-balneario', label: 'Balneário Camboriú (3-4 dias)' },
    { key: 'sc-beto-carrero', label: 'Beto Carrero World (2 dias)' },
    { key: 'sc-combo', label: 'Combo SC Completo (5 dias)' },
    { key: 'fretamento', label: 'Fretamento Personalizado' },
    { key: 'outro', label: 'Outro destino' },
]

const fieldBaseClass = '!mt-8'
const fieldLabelClass = '!start-0 !top-0 !-translate-y-[calc(100%+0.5rem)] font-medium text-brand-gunship'
const fieldWrapperClass = 'min-h-12 rounded-sm border border-brand-blue-grey bg-white text-brand-gunship transition-colors hover:border-brand-gunship focus-within:!border-brand-gunship focus-within:ring-2 focus-within:ring-brand-gunship focus-within:ring-offset-2 focus-within:ring-offset-white'
const fieldInputClass = 'text-brand-gunship placeholder:text-brand-gunship/80'
const fieldIconClass = 'text-brand-gunship/80'

const inputClassNames = {
    base: fieldBaseClass,
    input: fieldInputClass,
    inputWrapper: fieldWrapperClass,
    label: fieldLabelClass,
}

export default function BookingForm({ defaultDestination = '', onSuccess }) {
    const [formData, setFormData] = useState({
        name: '',
        phone: '',
        email: '',
        destination: defaultDestination,
        date: '',
        passengers: '',
        message: '',
    })
    const [isSubmitting, setIsSubmitting] = useState(false)
    const [isSubmitted, setIsSubmitted] = useState(false)

    const handleChange = (field, value) => {
        setFormData(prev => ({ ...prev, [field]: value }))
    }

    const formatWhatsAppMessage = () => {
        const destLabel = destinations.find(d => d.key === formData.destination)?.label || formData.destination
        return `Olá! Gostaria de fazer uma cotação:\n\n\n\n📍 *Destino:* ${destLabel}\n\n📅 *Data preferencial:* ${formData.date || 'A definir'}\n\n👥 *Passageiros:* ${formData.passengers || 'A definir'}\n\n\n\n👤 *Nome:* ${formData.name}\n\n📱 *Telefone:* ${formData.phone}\n\n📧 *Email:* ${formData.email}\n\n\n\n💬 *Mensagem:* ${formData.message || 'Aguardo retorno!'}`
    }

    const handleSubmit = (event) => {
        event.preventDefault()
        setIsSubmitting(true)

        const message = formatWhatsAppMessage()
        const contact = resolveBookingContact(formData.destination)
        window.open(createWhatsAppUrl(contact, message), '_blank')

        setTimeout(() => {
            setIsSubmitting(false)
            setIsSubmitted(true)
            onSuccess?.()
        }, 1000)
    }

    if (isSubmitted) {
        return (
            <Motion.div
                initial={{ opacity: 0, y: 12 }}
                animate={{ opacity: 1, y: 0 }}
                className="py-12 text-center"
                role="status"
                aria-live="polite"
            >
                <div className="mx-auto mb-6 flex h-20 w-20 items-center justify-center border border-brand-blue-grey bg-brand-lynx">
                    <CheckCircle size={40} className="text-brand-gunship" />
                </div>
                <h3 className="mb-2 text-2xl font-bold text-brand-gunship">Mensagem Enviada!</h3>
                <p className="mb-6 text-brand-gunship/80">
                    Você será redirecionado para o WhatsApp. Nossa equipe responderá em breve!
                </p>
                <Button
                    variant="bordered"
                    onClick={() => setIsSubmitted(false)}
                    className="min-h-11 rounded-sm border-brand-gunship bg-transparent font-semibold text-brand-gunship focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-gunship focus-visible:ring-offset-2 focus-visible:ring-offset-white"
                >
                    Enviar nova mensagem
                </Button>
            </Motion.div>
        )
    }

    return (
        <Motion.form
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            onSubmit={handleSubmit}
            className="space-y-10"
        >
            <Input
                label="Nome completo"
                labelPlacement="outside"
                variant="bordered"
                placeholder="Seu nome"
                value={formData.name}
                onChange={(event) => handleChange('name', event.target.value)}
                startContent={<User size={18} className={fieldIconClass} />}
                isRequired
                classNames={inputClassNames}
            />

            <div className="grid grid-cols-1 gap-x-6 gap-y-10 md:grid-cols-2">
                <Input
                    label="WhatsApp"
                    labelPlacement="outside"
                    variant="bordered"
                    placeholder="(49) 99999-9999"
                    value={formData.phone}
                    onChange={(event) => handleChange('phone', event.target.value)}
                    startContent={<Phone size={18} className={fieldIconClass} />}
                    isRequired
                    classNames={inputClassNames}
                />
                <Input
                    label="E-mail"
                    labelPlacement="outside"
                    variant="bordered"
                    type="email"
                    placeholder="seu@email.com"
                    value={formData.email}
                    onChange={(event) => handleChange('email', event.target.value)}
                    startContent={<Mail size={18} className={fieldIconClass} />}
                    classNames={inputClassNames}
                />
            </div>

            <Select
                label="Destino desejado"
                labelPlacement="outside"
                variant="bordered"
                placeholder="Selecione o destino"
                selectedKeys={formData.destination ? [formData.destination] : []}
                onChange={(event) => handleChange('destination', event.target.value)}
                startContent={<MapPin size={18} className={fieldIconClass} />}
                isRequired
                classNames={{
                    base: fieldBaseClass,
                    label: fieldLabelClass,
                    trigger: fieldWrapperClass,
                    value: 'text-brand-gunship group-data-[has-value=false]:text-brand-gunship/80',
                }}
            >
                {destinations.map((destination) => (
                    <SelectItem key={destination.key} value={destination.key}>
                        {destination.label}
                    </SelectItem>
                ))}
            </Select>

            <div className="grid grid-cols-1 gap-x-6 gap-y-10 md:grid-cols-2">
                <Input
                    label="Data preferencial"
                    labelPlacement="outside"
                    variant="bordered"
                    type="date"
                    value={formData.date}
                    onChange={(event) => handleChange('date', event.target.value)}
                    startContent={<Calendar size={18} className={fieldIconClass} />}
                    classNames={inputClassNames}
                />
                <Input
                    label="Número de passageiros"
                    labelPlacement="outside"
                    variant="bordered"
                    type="number"
                    placeholder="Ex: 40"
                    min="1"
                    value={formData.passengers}
                    onChange={(event) => handleChange('passengers', event.target.value)}
                    startContent={<Users size={18} className={fieldIconClass} />}
                    classNames={inputClassNames}
                />
            </div>

            <Textarea
                label="Observações"
                labelPlacement="outside"
                variant="bordered"
                placeholder="Conte-nos mais sobre sua viagem... (opcional)"
                value={formData.message}
                onChange={(event) => handleChange('message', event.target.value)}
                minRows={3}
                classNames={inputClassNames}
            />

            <Button
                type="submit"
                size="lg"
                fullWidth
                isLoading={isSubmitting}
                className="h-14 rounded-sm bg-brand-ember font-bold text-black transition-[filter] hover:brightness-95 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-gunship focus-visible:ring-offset-2 focus-visible:ring-offset-white active:brightness-90"
            >
                {isSubmitting ? 'Enviando...' : (
                    <>
                        <MessageCircle size={20} className="mr-2" />
                        Enviar via WhatsApp
                    </>
                )}
            </Button>

            <p className="text-center text-xs text-brand-gunship/80">
                Ao enviar, você será redirecionado para o WhatsApp com sua mensagem pré-formatada.
            </p>
        </Motion.form>
    )
}
