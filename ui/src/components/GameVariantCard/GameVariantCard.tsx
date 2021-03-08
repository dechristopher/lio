import React, {CSSProperties, FC} from 'react';
import {BackGroundColors, BorderColors, TextColors} from '@utils/styles/colors'
import classNames from "classnames";

interface GameVariantCardProps {
    bgColor: BackGroundColors;
    textColor: TextColors;
    width?: number;
    height?: number;
    style?: CSSProperties;
    className?: string;
    borderColor?: BorderColors;
    borderRadius?: number;
}

/**
 * Card containing variations for both preset and custom games.
 *
 * @param {GameVariantCardProps} props - game variant card props
 * @param {BackGroundColors} props.bgColor - card background color
 * @param {TextColors} props.textColor - card text color
 * @param {number} props.width - card width
 * @param {number} props.height - card height
 * @param {CSSProperties} props.style - card CSS style
 * @param {string} props.className - card class names
 * @param {BorderColors} props.borderColor - card border colors
 * @param {number} props.borderRadius - card border radius
 *
 * @returns {Element} - game variant card component
 *
 * @example
 * <GameVariantCard
 *       className="w-full"
 *       bgColor={bgColors.yellow["300"]}
 *       textColor={textColors.black["1000"]}
 *       width={40}
 *       height={40}
 *       style={{display: "flex"}}
 *       borderColor={borderColors.black["100"]}
 *       borderRadius={5}
 *   >
 *      // <CARD CONTENT>
 * </GameVariantCard>
 */
export const GameVariantCard: FC<GameVariantCardProps> = (props) => {
    const {height, bgColor, textColor, borderColor, children} = props

    return (
        <div
            className={classNames(
                bgColor,
                textColor,
                borderColor,
                props.className,
                "flex justify-center items-center"
            )}
            style={{
                height,
                borderRadius: props.borderRadius,
                ...props.style
            }}
        >
            {children}
        </div>
    )
}

GameVariantCard.defaultProps = {
    height: 125,
    borderRadius: 5,
    className: ""
}
