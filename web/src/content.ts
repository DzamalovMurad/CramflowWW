/**
 * Все тексты интерфейса в одном месте — меняйте здесь, не трогая компоненты.
 * Стиль бренда: всё строчными буквами (как задано в дизайне).
 * Описания и цены товаров живут в БД и редактируются через админ-бота (/add, /edit).
 */
export const content = {
  brand: 'cramflow',

  home: {
    title: 'цветы, которые хочется дарить',
    subtitle: 'свежие букеты, собранные вручную. доставка сегодня.',
    cta: 'перейти в каталог',
  },

  catalog: {
    title: 'каталог',
    priceFrom: 'от',
    addToCart: 'в корзину',
    empty: 'по этим условиям букетов не нашлось — попробуйте изменить фильтры',
  },

  product: {
    sizeLabel: 'размер букета',
    qtyLabel: 'количество',
    addToCart: 'в корзину',
    flowersUnit: 'шт',
    availabilityNote: 'состав можно согласовать на этапе сборки',
  },

  cart: {
    title: 'корзина',
    empty: 'корзина пуста',
    emptyHint: 'самое время выбрать букет',
    toCatalog: 'в каталог',
    inBouquet: 'в букете',
    total: 'итого',
    checkout: 'оформить заказ',
  },

  checkout: {
    title: 'оформление',
    name: 'ваше имя',
    namePlaceholder: 'иван',
    phone: 'телефон',
    phonePlaceholder: '+7 900 000-00-00',
    address: 'адрес доставки',
    addressPlaceholder: 'улица, дом, квартира',
    date: 'дата доставки',
    time: 'время доставки',
    comment: 'комментарий',
    commentOptional: 'необязательно',
    commentPlaceholder: 'например: без сирени',
    cardText: 'текст открытки',
    cardTextOptional: 'необязательно',
    cardTextPlaceholder: 'пожелание или посвящение',
    anonymous: 'отправить анонимно',
    promo: 'промокод',
    promoPlaceholder: 'WELCOME10',
    promoApply: 'применить',
    promoRemove: 'убрать',
    promoDiscount: 'скидка',
    subtotal: 'сумма',
    discount: 'скидка',
    total: 'итого',
    submit: 'подтвердить',
    submitting: 'отправляем…',
  },

  confirmation: {
    title: 'спасибо за ваш заказ!',
    orderLabel: 'заказ',
    orderAccepted: 'принят',
    composition: 'состав заказа',
    promoApplied: 'промокод применён',
    total: 'итого',
    delivery: 'доставка',
    managerCall: 'ожидайте звонка менеджера для подтверждения заказа',
    backToCatalog: 'вернуться в каталог',
  },
} as const;

/** Категории каталога: подписи строчными, значения — как в БД. */
export const categoryLabels: Record<string, string> = {
  Стандарт: 'стандарт',
  Премиум: 'премиум',
  Люкс: 'люкс',
  WOW: 'wow',
};

/** Быстрые фильтры каталога. */
export const filterLabels: { id: string; label: string }[] = [
  { id: 'popular', label: 'популярное' },
  { id: 'new', label: 'новинки' },
  { id: 'today', label: 'доставка сегодня' },
  { id: 'budget', label: 'до 3 000 ₽' },
];
