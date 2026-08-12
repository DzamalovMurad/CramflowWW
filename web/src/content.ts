/**
 * Все тексты интерфейса в одном месте — меняйте здесь, не трогая компоненты.
 * Стиль бренда: всё строчными буквами (как задано в дизайне).
 * Описания и цены товаров живут в БД и редактируются через админ-бота (/add, /edit).
 */
export const content = {
  brand: 'Flowix',

  common: {
    retry: 'попробовать ещё раз',
    close: 'закрыть',
    offline: 'нет связи — проверьте интернет и повторите',
  },

  error: {
    title: 'что-то пошло не так',
    hint: 'попробуйте обновить экран — корзина сохранится.',
    reload: 'обновить',
    toHome: 'на главную',
  },

  home: {
    badge: 'доставка по москве · сегодня',
    /** Заявление первого экрана: коротко и без вежливости. */
    title: 'Когда слова уже не нужны',
    subtitle: 'свежие букеты, собранные вручную. привезём сегодня.',
    heroCta: 'выбрать букет',
    freshToday: 'сегодня на базе',
    sectionTitle: 'букеты',
    shelfTitle: 'wow-букеты',
    shelfCaption: 'составные композиции, которые запоминают надолго',
    shelfAll: 'все',
    activeOrder: 'ваш заказ',
    dealsTitle: 'подешевле',
    dealsHint: 'в наличии сегодня',
  },

  /**
   * Подписи бейджей. CSS поднимает их в капс (.badge), поэтому здесь
   * строчные — как и остальные тексты интерфейса.
   */
  badge: {
    sale: 'акция',
    fresh: 'свежее',
    daily: 'букет дня',
    hit: 'хит',
    stock: 'осталось',
  },

  /** Сезонная кампания «к 1 сентября» (см. src/seasonal.ts). */
  seasonal: {
    pill: 'к 1 сентября',
    badge: '1 сентября',
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
    loadFailed: 'не удалось загрузить каталог',
    soldOut: 'нет в наличии',
    lastLeft: 'осталось',
  },

  product: {
    sizeLabel: 'размер букета',
    /** Статусные строки люксовых категорий — характер вместо ещё одного абзаца. */
    luxLine: 'классика. без компромиссов',
    wowLine: 'один жест — и больше слов не нужно',
    boxTitle: 'фирменная коробка flowix',
    boxNote: 'доедет таким, каким вы его видите',
    inCart: 'добавлено',
    qtyLabel: 'количество',
    addToCart: 'забрать',
    flowersUnit: 'шт',
    availabilityNote: 'состав согласуем на сборке',
    soldOut: 'этот букет закончился',
    soldOutHint: 'посмотрите другие — привезём сегодня',
    toCatalog: 'в каталог',
  },

  cart: {
    title: 'корзина',
    empty: 'корзина пуста',
    emptyHint: 'самое время выбрать букет',
    toCatalog: 'в каталог',
    inBouquet: 'в букете',
    total: 'итого',
    checkout: 'оформить',
    removed: 'убрали из корзины то, что закончилось',
  },

  checkout: {
    title: 'оформление',
    contactsSection: 'контакты',
    deliverySection: 'доставка',
    extrasSection: 'дополнительно',
    extrasHint: 'открытка, комментарий, промокод',

    name: 'ваше имя',
    namePlaceholder: 'иван',
    phone: 'телефон',
    phonePlaceholder: '+7 900 000-00-00',
    address: 'адрес доставки',
    addressPlaceholder: 'улица, дом, квартира',

    forSomeoneElse: 'доставить другому человеку',
    forSomeoneElseHint: 'курьер позвонит получателю, а не вам',
    recipientName: 'имя получателя',
    recipientNamePlaceholder: 'мама',
    recipientPhone: 'телефон получателя',

    date: 'дата доставки',
    dateToday: 'сегодня',
    dateTomorrow: 'завтра',
    dateOther: 'другой день',
    time: 'время доставки',
    timeExpress: 'в течение часа',
    timeAt: 'ко времени',
    timeAtLabel: 'к какому времени',
    timeNote: 'доставляем ежедневно с {open} до {close}',
    expressUnavailable: 'сегодня доставить уже не успеем — выберите другой день',

    comment: 'комментарий',
    commentOptional: 'необязательно',
    commentPlaceholder: 'например: без сирени, домофон 15',
    cardText: 'текст открытки',
    cardTextOptional: 'бесплатно',
    cardTextPlaceholder: 'пожелание или посвящение',
    anonymous: 'не называть отправителя',

    promo: 'промокод',
    promoPlaceholder: 'WELCOME10',
    promoApply: 'применить',
    promoRemove: 'убрать',
    promoFrom: 'скидка действует на заказ от',

    subtotal: 'сумма',
    discount: 'скидка',
    total: 'итого',
    submit: 'подтвердить заказ',
    submitting: 'отправляем…',
    submitRetry: 'отправить ещё раз',
    fixErrors: 'проверьте выделенные поля',
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
    repeat: 'повторить',
    repeatDone: 'добавили в корзину',
    about: 'о проекте',
    aboutText:
      'flowix — свежие авторские букеты с доставкой по москве. собираем вручную в день доставки.',
  },

  order: {
    number: 'заказ',
    composition: 'состав заказа',
    delivery: 'доставка',
    promoApplied: 'промокод применён',
    total: 'итого',
    cancelReason: 'причина отмены',
    notFound: 'заказ не найден',
  },

  confirmation: {
    title: 'заказ принят',
    subtitle: 'спасибо за доверие',
    managerCall: 'мы подтвердим заказ в этом чате — уведомления придут от бота',
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

/** Быстрые фильтры каталога (id совпадают с repository.Filter* на сервере). */
export const filterLabels: { id: string; label: string }[] = [
  { id: 'popular', label: 'популярное' },
  { id: 'new', label: 'новинки' },
  { id: 'preorder', label: 'предзаказ' },
  { id: 'budget', label: 'до 3 000 ₽' },
];

/** Подписи статусов заказа. Значения совпадают с model.StatusLabels на сервере. */
export const statusLabels: Record<string, string> = {
  new: 'принят',
  confirmed: 'подтверждён',
  assembling: 'собираем букет',
  delivering: 'курьер в пути',
  delivered: 'доставлен',
  cancelled: 'отменён',
};

/** Статусы, при которых заказ ещё «живой» и его стоит показывать на главной. */
export const ACTIVE_STATUSES = ['new', 'confirmed', 'assembling', 'delivering'];

/** Подстановка значений в шаблон: fill('с {open} до {close}', {open, close}). */
export function fill(template: string, values: Record<string, string | number>): string {
  return template.replace(/\{(\w+)\}/g, (_, key) => String(values[key] ?? ''));
}
