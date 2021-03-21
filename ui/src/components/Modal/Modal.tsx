import React, {FC, ReactNode, useEffect, useState} from 'react';
import {Transition} from "@headlessui/react";
import {ModalContextActions, useModalContext} from "@app/contexts/ModalContext";
import classNames from "classnames";

interface ModalProps {
    isOpen: boolean;
    content: ReactNode;
    hugContents?: boolean;
}

const modalZIndex = 40;

export const Modal: FC<ModalProps> = (props) => {
    const [, modalDispatch] = useModalContext()
    const [zIndex, setZIndex] = useState(-modalZIndex)

    /**
     * When opening, sets the zIndex to show content above everything else.
     * When closing, uses a timeout to set the zIndex after the modal has animated out.
     */
    useEffect(() => {
        if (props.isOpen) {
            setZIndex(modalZIndex)
        } else if (!props.isOpen) {
            setTimeout(() => {
                setZIndex(-modalZIndex)
                // clears the modal content
                modalDispatch({
                    type: ModalContextActions.SetContent,
                    payload: undefined
                })
            }, 300);
        }
    }, [props.isOpen, setZIndex, zIndex]);

    return (
        <div
            className={classNames(
                "fixed inset-0 overflow-y-auto",
            )}
            style={{
                zIndex
            }}
        >
            <div className={"flex items-end justify-center min-h-screen px-4 pb-20 text-center sm:block sm:p-0"}>
                <Transition
                    show={props.isOpen}
                    enter="ease-out duration-300"
                    enterFrom="opacity-0"
                    enterTo="opacity-100"
                    leave="ease-in duration-300"
                    leaveFrom="opacity-100"
                    leaveTo="opacity-0"
                >
                    {/* Overlay */}
                    <div className="fixed inset-0 transition-opacity" aria-hidden="true">
                        <div className="absolute inset-0 bg-gray-500 opacity-75"/>
                    </div>

                    {/* Centers content */}
                    <span className="hidden sm:inline-block sm:align-middle sm:h-screen"
                          aria-hidden="true">&#8203;</span>

                    {/* Modal container */}
                    <Transition
                        show={props.isOpen}
                        enter="ease-out duration-300"
                        enterFrom="opacity-0 translate-y-4 sm:translate-y-0 sm:scale-95"
                        enterTo="opacity-100 translate-y-0 sm:scale-100"
                        leave="ease-in duration-300"
                        leaveFrom="opacity-100 translate-y-0 sm:scale-100"
                        leaveTo="opacity-0 translate-y-4 sm:translate-y-0 sm:scale-95"
                    >
                        <div
                            style={{
                                position: "absolute",
                                top: "50%",
                                left: "50%",
                                margin: 0,
                                transform: "translate(-50%, -50%)",
                                overflow: "visible"
                            }}
                            className="inline-block align-bottom bg-white rounded-lg px-4 pt-5 pb-4 text-left overflow-hidden shadow-xl transform transition-all sm:align-middle sm:max-w-lg sm:w-full sm:p-6"
                            role="dialog" aria-modal="true" aria-labelledby="modal-headline">
                            {/* Close button */}
                            <div className="block absolute top-0 right-0 pt-4 pr-4">
                                <button
                                    type="button"
                                    onClick={() => {
                                        modalDispatch({
                                            type: ModalContextActions.SetIsOpen,
                                            payload: false
                                        })
                                    }}
                                    className="bg-white rounded-md text-gray-400 hover:text-gray-500 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500">
                                    <span className="sr-only">Close</span>

                                    <svg className="h-6 w-6" xmlns="http://www.w3.org/2000/svg" fill="none"
                                         viewBox="0 0 24 24" stroke="currentColor" aria-hidden="true">
                                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2"
                                              d="M6 18L18 6M6 6l12 12"/>
                                    </svg>
                                </button>
                            </div>

                            {/* Modal content */}
                            <div className="sm:flex sm:items-start">
                                {props.content}
                            </div>
                        </div>
                    </Transition>
                </Transition>
            </div>
        </div>
    )
}
