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
  },

  /**
   * Доставка. Вариантов два: до метро (бесплатно, входит в цену букета)
   * и по адресу (курьер Яндекса — цену называет менеджер после заказа).
   * Стоимость курьера приложение не считает и нигде не показывает,
   * поэтому в итог заказа доставка не добавляется никогда.
   * Формулировки metroLine/addressLine/addressNote согласованы с админ-ботом —
   * менять их нужно в обоих местах сразу.
   */
  delivery: {
    label: 'способ доставки',
    metroOption: 'до метро',
    metroOptionNote: 'бесплатно',
    addressOption: 'по адресу',
    addressOptionNote: 'курьер, оплачивается отдельно',
    metroStation: 'станция метро',
    metroPlaceholder: 'начните вводить название',
    metroNotFound: 'станция не найдена — проверьте название',
    metroHint: 'курьер передаст букет у выхода из метро',
    metroChange: 'изменить',
    addressNote:
      'Стоимость доставки курьером зависит от адреса. Менеджер свяжется с вами, назовёт цену и согласует время. Букет оплачивается отдельно от доставки.',
    cartFree: 'Доставка до метро — бесплатно',
    cartFreeHint: 'или курьером по адресу — стоимость назовёт менеджер',
    metroLine: 'Доставка до метро {station} — бесплатно',
    addressLine: 'Доставка по адресу — менеджер свяжется и назовёт стоимость курьера',
    timeManaged: 'время согласует менеджер',
    timeNotSet: 'время не указано',
    totalNoteMetro: 'доставка до метро — бесплатно',
    totalNoteAddress: 'букет оплачивается отдельно от доставки курьером',
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
    timeOptional: 'необязательно',
    timeNote: 'доставляем ежедневно с 9:00 до 21:00',
    timeAtLabel: 'к какому времени',
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
    about: 'о проекте',
    aboutText: 'flowix — свежие авторские букеты с доставкой по москве. собираем вручную в день доставки.',
    history: 'мои заказы',
    historyEmpty: 'заказов пока нет',
  },

  confirmation: {
    title: 'заказ успешно добавлен',
    orderLabel: 'заказ',
    orderAccepted: 'принят — спасибо за доверие',
    composition: 'состав заказа',
    promoApplied: 'промокод применён',
    total: 'итого',
    managerCall: 'ожидайте звонка менеджера для подтверждения заказа',
    backToCatalog: 'вернуться в каталог',
  },
} as const;

/** Статусы заказа для истории в профиле — те же, что в админ-боте. */
export const orderStatusLabels: Record<string, string> = {
  new: '🆕 новый',
  confirmed: '✅ подтверждён',
  assembling: '💐 собираем',
  photo_sent: '📷 фото отправлено',
  delivering: '🚗 в пути',
  delivered: '🌸 доставлен',
  cancelled: '❌ отменён',
};

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
