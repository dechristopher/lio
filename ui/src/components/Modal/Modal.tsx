import React, {FC, ReactNode} from 'react';
import {ModalContextActions, useModalContext} from "@app/contexts/ModalContext";

interface ModalProps {
    isOpen: boolean;
    content: ReactNode;
    footerContent?: ReactNode;
}

export const Modal: FC<ModalProps> = (props) => {
    const [, modalDispatch] = useModalContext()

    return props.isOpen ? (
        <div className="fixed z-30 inset-0 overflow-y-auto">
            <div className="flex items-end justify-center min-h-screen pt-4 px-4 pb-20 text-center sm:block sm:p-0">
                {/* Overlay */}
                <div className="fixed inset-0 transition-opacity" aria-hidden="true">
                    <div className="absolute inset-0 bg-gray-500 opacity-75"/>
                </div>

                {/* Centers content */}
                <span className="hidden sm:inline-block sm:align-middle sm:h-screen" aria-hidden="true">&#8203;</span>

                {/* Modal container */}
                <div className="inline-block align-bottom bg-white rounded-lg px-4 pt-5 pb-4 text-left overflow-hidden shadow-xl transform transition-all sm:my-8 sm:align-middle sm:max-w-lg sm:w-full sm:p-6" role="dialog" aria-modal="true" aria-labelledby="modal-headline">
                    {/* Close button */}
                    <div className="hidden sm:block absolute top-0 right-0 pt-4 pr-4">
                        <button
                            type="button"
                            onClick={() => {
                                modalDispatch({
                                    type: ModalContextActions.SetContent,
                                    payload: undefined
                                })
                            }}
                            className="bg-white rounded-md text-gray-400 hover:text-gray-500 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500">
                            <span className="sr-only">Close</span>

                            <svg className="h-6 w-6" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" aria-hidden="true">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M6 18L18 6M6 6l12 12" />
                            </svg>
                        </button>
                    </div>

                    {/* Modal content */}
                    <div className="sm:flex sm:items-start">
                        {props.content}
                    </div>

                    {/* Footer content */}
                    {/*<div className="mt-5 sm:mt-4 sm:flex sm:flex-row-reverse">*/}
                    {/*    {props.footerContent}*/}
                    {/*</div>*/}
                </div>
            </div>
        </div>
    ) : null
}