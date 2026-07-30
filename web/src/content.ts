/**
 * Все тексты интерфейса в одном месте — меняйте здесь, не трогая компоненты.
 * Стиль бренда: всё строчными буквами (как задано в дизайне).
 * Описания и цены товаров живут в БД и редактируются через админ-бота (/add, /edit).
 */
export const content = {
  brand: 'flowix',

  home: {
    badge: 'доставка по москве · сегодня',
    title: 'цветы, которые хочется дарить',
    subtitle: 'свежие букеты, собранные вручную. предзаказ к нужному времени.',
    cta: 'выбрать букет',
    freshToday: 'сегодня на базе',
    sectionTitle: 'букеты',
    shelfTitle: 'wow-букеты',
    shelfCaption: 'составные композиции, которые запоминают надолго',
    shelfAll: 'все',
  },

  sort: {
    title: 'сортировка',
    default: 'по умолчанию',
    cheap: 'сначала дешевле',
    expensive: 'сначала дороже',
  },

  catalog: {
    title: 'каталог',
    priceFrom: 'от',
    addToCart: 'в корзину',
    deliveryToday: 'доставим сегодня',
    count: 'товаров',
    searchPlaceholder: 'поиск букетов',
    empty: 'по этим условиям букетов не нашлось — попробуйте изменить фильтры',
    nothingFound: 'по запросу ничего не найдено',
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
    addons: 'добавить к заказу',
    size: 'размер',
    repeatMissing: 'не удалось добавить',
    promoBanner: 'промокод {code} применится при оформлении',
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
    time: 'слот доставки',
    timeNote: 'слоты по 2 часа, заказы на сегодня — до 19:00',
    slotsEmpty: 'нет доступных слотов — попробуйте другую дату',
    notSelf: 'заказываю не себе',
    recipientName: 'имя получателя',
    recipientNamePlaceholder: 'анна',
    recipientPhone: 'телефон получателя',
    addressByRecipient: 'адрес уточнит курьер у получателя',
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

  profile: {
    title: 'профиль',
    guest: 'вы ещё не оформляли заказов',
    guestHint: 'выберите букет — и ваши данные сохранятся здесь',
    name: 'имя',
    phone: 'телефон',
    promo: 'ваш промокод',
    discount: 'скидка',
    toCatalog: 'в каталог',
    orders: 'мои заказы',
    orderLabel: 'заказ',
    repeat: 'повторить заказ',
    repeating: 'собираем…',
    repeatEmpty: 'товары из заказа больше недоступны',
    about: 'о проекте',
    aboutText: 'flowix — свежие авторские букеты с доставкой по москве. собираем вручную в день доставки.',
  },

  confirmation: {
    title: 'заказ успешно добавлен',
    orderLabel: 'заказ',
    orderAccepted: 'принят — спасибо за доверие',
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
  { id: 'preorder', label: 'предзаказ' },
  { id: 'budget', label: 'до 3 000 ₽' },
];
