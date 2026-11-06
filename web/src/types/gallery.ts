export interface Photo {
  id: string
  name: string
  favorite: boolean
  thumbnail: string
  original: string
  mediaType: 'image' | 'video'
  aspectRatio: number
}

export interface PhotoGroup {
  publishedDate?: string
  id: string
  albumID: number
  name: string
  detail: string
  count: number
  favorite: boolean
  canFavorite: boolean
  people: RelatedPerson[]
  series: RelatedSeries[]
  photos: Photo[]
}

export interface RelatedSeries {
  id: number
  name: string
}

export interface RelatedPerson {
  id: number
  name: string
  note: string
}
