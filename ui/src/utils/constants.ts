import {bgColors} from "@utils/styles/colors";

export const BaseURL = process.env.NODE_ENV === "development" ?
    process.env.DEV_API_URL :
    process.env.PROD_API_URL;

// The rated pools that we have defined
export enum GamePools {
    Bullet = "BULLET",
    Blitz = "BLITZ",
    Rapid = "RAPID",
    Hyper = "HYPER",
    Ulti = "ULTI"
}

// Order in which we sort the rated pools by
export const PoolsOrder = [
    GamePools.Bullet,
    GamePools.Blitz,
    GamePools.Rapid,
    GamePools.Hyper,
    GamePools.Ulti
]

export const PoolColors = {
    [GamePools.Bullet]: bgColors.yellow["300"],
    [GamePools.Blitz]: bgColors.green["500"],
    [GamePools.Rapid]: bgColors.green["500"],
    [GamePools.Hyper]: bgColors.purple["400"],
    [GamePools.Ulti]: bgColors.purple["400"],
}

export enum Times {
    Zero = 0,
    One = 1,
    Three = 3,
    Five = 5,
    Ten = 10,
    Fifteen = 15,
    Thirty = 30,
    OneMin = 60,
    ThreeMin = 180,
    FiveMin = 300,
    TenMin = 600
}

export enum ColorOptions {
    White,
    Black,
    Random
}

export enum GameModes {
    PlayOnline,
    PlayAFriend,
    PlayComputer
}

export enum GameTypes {
    RATED = "Rated",
    CASUAL = "Casual"
}

export enum FontWeights {
    Hairline = "font-hairline",
    Thin = "font-thin",
    Light = "font-light",
    Normal = "font-normal",
    Medium = "font-medium",
    SemiBold = "font-semibold",
    Bold = "font-bold",
    ExtraBold = "font-extrabold",
    Black = "font-black"
}

export enum FontSizes {
    XS = "text-xs",
    S = "text-sm",
    M = "text-base",
    LG = "text-lg",
    XL = "text-xl",
    TwoXL = "text-2xl",
    ThreeXL = "text-3xl",
    FourXL = "text-4xl",
    FiveXL = "text-5xl",
    SixXL = "text-6xl",
}