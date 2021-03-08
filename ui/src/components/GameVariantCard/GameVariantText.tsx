import React, {FC} from 'react'
import {FontWeights} from "@utils/constants";
import classNames from "classnames";

interface GameVariantTextProps {
    timeControl: string;
    variantName?: string;
    timeControlFontWeight?: FontWeights
    variantNameFontWeight?: FontWeights;
}

/**
 * Text component for displaying a given game variation.
 *
 * @param {GameVariantTextProps} props - game variant text props
 * @param {string} props.timeControl - game time control
 * @param {string} props.variantName - game variant name
 * @param {FontWeights} props.timeControlFontWeights - time control font weight
 * @param {FontWeights} props.variantNameFontWeights - variant name font weight
 *
 * @returns {Element} - game variant text component
 *
 * @example
 * <GameVariantText
 *   timeControl="1 + 0"
 *   variantName="Rapid"
 *   timeControlFontWeight={FontWeights.medium}
 *   variantNameFontWeight={FontWeights.semibold}
 * />
 */
export const GameVariantText: FC<GameVariantTextProps> = (props) => {
    return (
        <div className="flex flex-col text-center md:p-4">
            <span className={
                classNames(
                    "text-4xl",
                    props.timeControlFontWeight
                )}
            >
                {props.timeControl}
            </span>
            {props.variantName ?
                <span className={
                    classNames(
                        "text-2xl",
                        props.variantNameFontWeight
                    )}>
                    {props.variantName}
                </span> : null}
        </div>
    )
}

GameVariantText.defaultProps = {
    timeControlFontWeight: FontWeights.semibold,
    variantNameFontWeight: FontWeights.semibold
}